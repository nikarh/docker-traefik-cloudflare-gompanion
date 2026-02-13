package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
)

type Config struct {
	DryRun                        bool
	DefaultTTL                    int
	EnableDockerPoll              bool
	DockerSwarmMode               bool
	EnableTraefikPoll             bool
	TraefikPollInsecureSkipVerify bool
	RefreshEntries                bool
	TraefikFilter                 *regexp.Regexp
	TraefikFilterRaw              string
	TraefikFilterKey              *regexp.Regexp
	TraefikPollSecs               int
	TraefikPollURL                string
	TraefikPollCACertFile         string
	TraefikVersion                string
	RecordType                    string
	TargetDomain                  string
	Domains                       []DomainConfig
	IncludedHosts                 []*regexp.Regexp
	ExcludedHosts                 []*regexp.Regexp
	CloudflareEmail               string
	CloudflareToken               string
	LogLevel                      string
	DockerCACertFile              string
	DockerInsecureSkipVerify      bool
}

type DomainConfig struct {
	Name               string
	Proxied            bool
	ZoneID             string
	TTL                int
	TargetDomain       string
	Comment            string
	ExcludedSubDomains []string
}

var defaultSecretDirs = []string{"/run/secrets"}

type Companion struct {
	cfg     Config
	cf      *CloudflareAPI
	docker  *client.Client
	synced  map[string]int
	syncedM sync.Mutex
}

func main() {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	logger := NewLogger(cfg.LogLevel)

	cf, err := NewCloudflareAPI(cfg.CloudflareEmail, cfg.CloudflareToken, logger)
	if err != nil {
		logger.Errorf("failed to initialize cloudflare api: %v", err)
		os.Exit(1)
	}

	comp := &Companion{
		cfg:    cfg,
		cf:     cf,
		synced: map[string]int{},
	}

	if cfg.EnableDockerPoll {
		dockerOpts := []client.Opt{
			client.FromEnv,
			client.WithAPIVersionNegotiation(),
		}
		if dockerHTTPClient, ok, err := newDockerHTTPClient(cfg); err != nil {
			logger.Errorf("failed to configure docker tls options: %v", err)
			os.Exit(1)
		} else if ok {
			dockerOpts = append(dockerOpts, client.WithHTTPClient(dockerHTTPClient))
		}

		dockerClient, err := client.NewClientWithOpts(dockerOpts...)
		if err != nil {
			logger.Errorf("failed to initialize docker client: %v", err)
			os.Exit(1)
		}
		comp.docker = dockerClient
	}

	if cfg.DryRun {
		logger.Warnf("Dry Run: %v", cfg.DryRun)
	}
	logger.Debugf("Docker Polling: %v", cfg.EnableDockerPoll)
	logger.Debugf("Swarm Mode: %v", cfg.DockerSwarmMode)
	logger.Debugf("Refresh Entries: %v", cfg.RefreshEntries)
	logger.Debugf("Traefik Version: %s", cfg.TraefikVersion)
	logger.Debugf("Default TTL: %d", cfg.DefaultTTL)

	if cfg.EnableTraefikPoll {
		logger.Debugf("Traefik Poll Url: %s", cfg.TraefikPollURL)
		logger.Debugf("Traefik Poll Seconds: %d", cfg.TraefikPollSecs)
		logger.Debugf("Traefik Poll CA Cert File: %s", cfg.TraefikPollCACertFile)
		logger.Debugf("Traefik Poll Insecure Skip Verify: %v", cfg.TraefikPollInsecureSkipVerify)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	initialMappings := map[string]int{}
	runWithRecover(logger, "initial-mapping", func() {
		mappings, err := comp.GetInitialMappings(ctx, logger)
		if err != nil {
			logger.Errorf("failed to get initial mappings: %v", err)
			return
		}
		initialMappings = mappings
	})
	comp.SyncMappings(initialMappings, logger)

	wg := &sync.WaitGroup{}
	if cfg.EnableTraefikPoll {
		wg.Add(1)
		go func() {
			defer wg.Done()
			comp.RunTraefikPoller(ctx, logger)
		}()
	}

	if cfg.EnableDockerPoll {
		wg.Add(1)
		go func() {
			defer wg.Done()
			comp.RunDockerEventWatch(ctx, logger)
		}()
	}

	<-ctx.Done()
	wg.Wait()
}

func LoadConfigFromEnv() (Config, error) {
	cfg := Config{}
	cfg.DryRun = parseBoolLikePython(os.Getenv("DRY_RUN"), false)
	cfg.DefaultTTL = parseIntOr(os.Getenv("DEFAULT_TTL"), 1)
	cfg.EnableDockerPoll = parseBoolLikePython(os.Getenv("ENABLE_DOCKER_POLL"), true)
	cfg.DockerSwarmMode = parseBoolLikePython(os.Getenv("DOCKER_SWARM_MODE"), false)
	cfg.EnableTraefikPoll = parseBoolLikePython(os.Getenv("ENABLE_TRAEFIK_POLL"), false)
	cfg.TraefikPollInsecureSkipVerify = parseBoolLikePython(os.Getenv("TRAEFIK_POLL_INSECURE_SKIP_VERIFY"), false)
	cfg.RefreshEntries = parseBoolLikePython(os.Getenv("REFRESH_ENTRIES"), false)
	cfg.LogLevel = defaultString(os.Getenv("LOG_LEVEL"), "INFO")
	cfg.TraefikPollSecs = parseIntOr(os.Getenv("TRAEFIK_POLL_SECONDS"), 60)
	cfg.TraefikPollURL = os.Getenv("TRAEFIK_POLL_URL")
	cfg.TraefikPollCACertFile = os.Getenv("TRAEFIK_POLL_CA_CERT_FILE")
	cfg.TraefikVersion = defaultString(os.Getenv("TRAEFIK_VERSION"), "2")
	cfg.DockerCACertFile = os.Getenv("DOCKER_CA_CERT_FILE")
	cfg.DockerInsecureSkipVerify = parseBoolLikePython(os.Getenv("DOCKER_INSECURE_SKIP_VERIFY"), false)
	cfg.RecordType = defaultString(os.Getenv("RC_TYPE"), "CNAME")
	cfg.TargetDomain = os.Getenv("TARGET_DOMAIN")

	filterLabel := defaultString(os.Getenv("TRAEFIK_FILTER_LABEL"), "traefik.constraint")
	labelRegex, err := regexp.Compile(filterLabel)
	if err != nil {
		return cfg, fmt.Errorf("invalid TRAEFIK_FILTER_LABEL regex: %w", err)
	}
	cfg.TraefikFilterKey = labelRegex

	if filterRaw := os.Getenv("TRAEFIK_FILTER"); filterRaw != "" {
		cfg.TraefikFilterRaw = filterRaw
		filterRegex, err := regexp.Compile(filterRaw)
		if err != nil {
			return cfg, fmt.Errorf("invalid TRAEFIK_FILTER regex: %w", err)
		}
		cfg.TraefikFilter = filterRegex
	}

	cfg.CloudflareEmail = getSecretByEnv("CF_EMAIL")
	cfg.CloudflareToken = getSecretByEnv("CF_TOKEN")
	if cfg.CloudflareToken == "" {
		return cfg, errors.New("CF_TOKEN not defined")
	}
	if cfg.TargetDomain == "" {
		return cfg, errors.New("TARGET_DOMAIN not defined")
	}

	domains, err := loadDomainConfigs(cfg.DefaultTTL, cfg.TargetDomain)
	if err != nil {
		return cfg, err
	}
	if len(domains) == 0 {
		return cfg, errors.New("DOMAIN1 not defined")
	}
	cfg.Domains = domains

	if !cfg.EnableDockerPoll && cfg.DockerSwarmMode {
		return cfg, errors.New("cannot enable DOCKER_SWARM_MODE without ENABLE_DOCKER_POLL=true")
	}

	included, excluded, err := loadTraefikHostFilters()
	if err != nil {
		return cfg, err
	}
	cfg.IncludedHosts = included
	cfg.ExcludedHosts = excluded

	if cfg.EnableTraefikPoll {
		if cfg.TraefikVersion != "2" {
			cfg.EnableTraefikPoll = false
		} else if !validURI(cfg.TraefikPollURL) {
			cfg.EnableTraefikPoll = false
		}
	}

	return cfg, nil
}

func loadDomainConfigs(defaultTTL int, targetDomain string) ([]DomainConfig, error) {
	rxDoms := regexp.MustCompile(`(?i)^DOMAIN[0-9]+$`)
	keys := make([]string, 0)
	for _, key := range os.Environ() {
		name := strings.SplitN(key, "=", 2)[0]
		if rxDoms.MatchString(name) {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)

	doms := make([]DomainConfig, 0, len(keys))
	for _, key := range keys {
		name := os.Getenv(key)
		zone := getSecretByEnv(key + "_ZONE_ID")
		if zone == "" {
			return nil, fmt.Errorf("%s is not set", key+"_ZONE_ID")
		}
		ttl := parseIntOr(os.Getenv(key+"_TTL"), defaultTTL)
		target := defaultString(os.Getenv(key+"_TARGET_DOMAIN"), targetDomain)
		excluded := splitCleanCSV(os.Getenv(key + "_EXCLUDED_SUB_DOMAINS"))
		doms = append(doms, DomainConfig{
			Name:               name,
			Proxied:            parseBoolLikePython(os.Getenv(key+"_PROXIED"), false),
			ZoneID:             zone,
			TTL:                ttl,
			TargetDomain:       target,
			Comment:            os.Getenv(key + "_COMMENT"),
			ExcludedSubDomains: excluded,
		})
	}

	return doms, nil
}

func loadTraefikHostFilters() ([]*regexp.Regexp, []*regexp.Regexp, error) {
	rInc := regexp.MustCompile(`(?i)^TRAEFIK_INCLUDED_HOST[0-9]+$`)
	rExc := regexp.MustCompile(`(?i)^TRAEFIK_EXCLUDED_HOST[0-9]+$`)

	includes := make([]*regexp.Regexp, 0)
	excludes := make([]*regexp.Regexp, 0)
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		name := parts[0]
		val := ""
		if len(parts) == 2 {
			val = parts[1]
		}
		if rInc.MatchString(name) {
			re, err := regexp.Compile(val)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid %s regex: %w", name, err)
			}
			includes = append(includes, re)
		}
		if rExc.MatchString(name) {
			re, err := regexp.Compile(val)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid %s regex: %w", name, err)
			}
			excludes = append(excludes, re)
		}
	}

	if len(includes) == 0 {
		includes = append(includes, regexp.MustCompile(`.*`))
	}

	return includes, excludes, nil
}

func (c *Companion) GetInitialMappings(ctx context.Context, logger *Logger) (map[string]int, error) {
	mappings := map[string]int{}

	if c.cfg.EnableDockerPoll {
		containers, err := c.docker.ContainerList(ctx, container.ListOptions{})
		if err != nil {
			return nil, err
		}
		for _, ctr := range containers {
			json, err := c.docker.ContainerInspect(ctx, ctr.ID)
			if err != nil {
				continue
			}
			if c.cfg.TraefikVersion == "1" {
				addToMappings(mappings, c.checkContainerT1(json.ID, json.Config.Labels, logger))
			} else {
				addToMappings(mappings, c.checkContainerT2(json.ID, json.Config.Labels, logger))
			}
		}
	}

	if c.cfg.DockerSwarmMode {
		services, err := c.docker.ServiceList(ctx, swarm.ServiceListOptions{})
		if err != nil {
			return nil, err
		}
		for _, svc := range services {
			if c.cfg.TraefikVersion == "1" {
				if svc.Spec.TaskTemplate.ContainerSpec != nil {
					addToMappings(mappings, c.checkServiceT1(svc.ID, svc.Spec.TaskTemplate.ContainerSpec.Labels, logger))
				}
			} else {
				addToMappings(mappings, c.checkServiceT2(svc.ID, svc.Spec.Labels, logger))
			}
		}
	}

	if c.cfg.EnableTraefikPoll {
		addToMappings(mappings, c.checkTraefik(ctx, logger))
	}

	return mappings, nil
}

func (c *Companion) RunTraefikPoller(ctx context.Context, logger *Logger) {
	ticker := time.NewTicker(time.Duration(c.cfg.TraefikPollSecs) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runWithRecover(logger, "traefik-poller", func() {
				c.SyncMappings(c.checkTraefik(ctx, logger), logger)
			})
		}
	}
}

func (c *Companion) RunDockerEventWatch(ctx context.Context, logger *Logger) {
	since := strconv.FormatInt(time.Now().Unix(), 10)
	for {
		if ctx.Err() != nil {
			return
		}
		filterArgs := filters.NewArgs()
		filterArgs.Add("type", "container")
		filterArgs.Add("type", "service")
		eventCh, errCh := c.docker.Events(ctx, events.ListOptions{Since: since, Filters: filterArgs})
		for {
			select {
			case <-ctx.Done():
				return
			case err := <-errCh:
				if err != nil && !errors.Is(err, context.Canceled) {
					logger.Errorf("docker event watcher error: %v", err)
					time.Sleep(2 * time.Second)
				}
				goto reconnect
			case ev, ok := <-eventCh:
				if !ok {
					goto reconnect
				}
				runWithRecover(logger, "docker-event-watch", func() {
					since = strconv.FormatInt(ev.Time, 10)
					newMappings := c.processDockerEvent(ctx, ev, logger)
					c.SyncMappings(newMappings, logger)
				})
			}
		}
	reconnect:
	}
}

func (c *Companion) processDockerEvent(ctx context.Context, event events.Message, logger *Logger) map[string]int {
	newMappings := map[string]int{}
	evtType := event.Type
	evtAction := string(event.Action)

	if evtType == events.ContainerEventType && evtAction == "start" {
		contID := event.Actor.ID
		if contID == "" {
			logger.Debugf("Skip container event without id")
			return newMappings
		}
		json, err := c.docker.ContainerInspect(ctx, contID)
		if err == nil {
			if c.cfg.TraefikVersion == "1" {
				addToMappings(newMappings, c.checkContainerT1(json.ID, json.Config.Labels, logger))
			} else {
				addToMappings(newMappings, c.checkContainerT2(json.ID, json.Config.Labels, logger))
			}
		}
	}

	if c.cfg.DockerSwarmMode && evtType == events.ServiceEventType && evtAction == "update" {
		nodeID := event.Actor.ID
		if nodeID == "" {
			logger.Debugf("Skip service update event without Actor.ID")
			return newMappings
		}
		svc, _, err := c.docker.ServiceInspectWithRaw(ctx, nodeID, swarm.ServiceInspectOptions{})
		if err == nil {
			if c.cfg.TraefikVersion == "1" {
				if svc.Spec.TaskTemplate.ContainerSpec != nil {
					addToMappings(newMappings, c.checkServiceT1(nodeID, svc.Spec.TaskTemplate.ContainerSpec.Labels, logger))
				}
			} else {
				addToMappings(newMappings, c.checkServiceT2(nodeID, svc.Spec.Labels, logger))
			}
		}
	}

	return newMappings
}

func (c *Companion) checkContainerT1(id string, labels map[string]string, logger *Logger) map[string]int {
	mappings := map[string]int{}
	if !c.matchTraefikFilter(labels) {
		return mappings
	}
	for key, value := range labels {
		if regexp.MustCompile(`traefik.*.frontend.rule`).MatchString(key) {
			for _, host := range parseTraefikV1HostRule(value) {
				logger.Verbosef("Found Container ID: %s with Hostname %s", id, host)
				mappings[host] = 1
			}
		}
	}
	return mappings
}

func (c *Companion) checkServiceT1(id string, labels map[string]string, logger *Logger) map[string]int {
	mappings := map[string]int{}
	if !c.matchTraefikFilter(labels) {
		return mappings
	}
	for key, value := range labels {
		if regexp.MustCompile(`traefik.*.frontend.rule`).MatchString(key) {
			for _, host := range parseTraefikV1HostRule(value) {
				logger.Verbosef("Found Service ID: %s with Hostname %s", id, host)
				mappings[host] = 1
			}
		}
	}
	return mappings
}

func (c *Companion) checkContainerT2(id string, labels map[string]string, logger *Logger) map[string]int {
	mappings := map[string]int{}
	if !c.matchTraefikFilter(labels) {
		return mappings
	}
	for key, value := range labels {
		if regexp.MustCompile(`traefik.*?\.rule`).MatchString(key) && strings.Contains(value, "Host") {
			for _, host := range parseTraefikV2Rule(value) {
				logger.Verbosef("Found Service ID: %s with Hostname %s", id, host)
				mappings[host] = 1
			}
		}
	}
	return mappings
}

func (c *Companion) checkServiceT2(id string, labels map[string]string, logger *Logger) map[string]int {
	mappings := map[string]int{}
	if !c.matchTraefikFilter(labels) {
		return mappings
	}
	for key, value := range labels {
		if regexp.MustCompile(`traefik.*?\.rule`).MatchString(key) && strings.Contains(value, "Host") {
			for _, host := range parseTraefikV2Rule(value) {
				logger.Verbosef("Found Service ID: %s with Hostname %s", id, host)
				mappings[host] = 1
			}
		}
	}
	return mappings
}

func (c *Companion) checkTraefik(ctx context.Context, logger *Logger) map[string]int {
	mappings := map[string]int{}
	logger.Verbosef("Querying Traefik routers from %s", c.cfg.TraefikPollURL)
	routers, statusCode, body, err := FetchTraefikRouters(
		ctx,
		c.cfg.TraefikPollURL,
		c.cfg.TraefikPollInsecureSkipVerify,
		c.cfg.TraefikPollCACertFile,
	)
	if err != nil {
		logger.Errorf("failed to poll traefik routers: %v", err)
		return mappings
	}
	if statusCode != 200 {
		logger.Errorf("Traefik API returned error %d: %s", statusCode, body)
		return mappings
	}
	for _, router := range routers {
		if router.Status != "enabled" || router.Name == "" {
			continue
		}
		if !strings.Contains(router.Rule, "Host") {
			continue
		}
		extracted := parseTraefikRouterRule(router.Rule)
		for _, host := range extracted {
			if !isMatching(host, c.cfg.IncludedHosts) {
				continue
			}
			if isMatching(host, c.cfg.ExcludedHosts) {
				continue
			}
			logger.Verbosef("Found Traefik Router Name: %s with Hostname %s", router.Name, host)
			mappings[host] = 2
		}
	}
	return mappings
}

func (c *Companion) matchTraefikFilter(labels map[string]string) bool {
	if c.cfg.TraefikFilter == nil {
		return true
	}
	for key, value := range labels {
		if c.cfg.TraefikFilterKey.MatchString(key) && c.cfg.TraefikFilter.MatchString(value) {
			return true
		}
	}
	return false
}

func (c *Companion) SyncMappings(mappings map[string]int, logger *Logger) {
	for name, source := range mappings {
		c.syncedM.Lock()
		current, exists := c.synced[name]
		c.syncedM.Unlock()
		if exists && current <= source {
			continue
		}
		if c.pointDomain(name, logger) {
			c.syncedM.Lock()
			c.synced[name] = source
			c.syncedM.Unlock()
		}
	}
}

func (c *Companion) pointDomain(name string, logger *Logger) bool {
	ok := true
	for _, dom := range c.cfg.Domains {
		if name == dom.TargetDomain {
			continue
		}
		if !strings.Contains(name, dom.Name) {
			continue
		}
		if isDomainExcluded(name, dom) {
			logger.Verbosef("Ignoring %s because it falls under excluded sub domain", name)
			continue
		}

		records, err := c.cf.ListDNSRecords(dom.ZoneID, name)
		if err != nil {
			logger.Errorf("%s list dns records failed: %v", name, err)
			ok = false
			continue
		}
		if len(records) == 0 {
			logger.Verbosef("Domain %s: Cloudflare record exists=false, configuration change required=true", name)
		} else {
			requiresChange := c.cfg.RefreshEntries
			for _, rec := range records {
				if rec.Content != dom.TargetDomain {
					requiresChange = true
					break
				}
			}
			logger.Verbosef("Domain %s: Cloudflare record exists=true, configuration change required=%v", name, requiresChange)
		}

		data := DNSRecordRequest{
			Type:    c.cfg.RecordType,
			Name:    name,
			Content: dom.TargetDomain,
			TTL:     dom.TTL,
			Proxied: dom.Proxied,
			Comment: dom.Comment,
		}

		if len(records) == 0 {
			if c.cfg.DryRun {
				logger.Infof("DRY-RUN: POST to Cloudflare %s: %+v", dom.ZoneID, data)
			} else {
				if err := c.cf.CreateDNSRecord(dom.ZoneID, data); err != nil {
					logger.Errorf("%s create record failed: %v", name, err)
					ok = false
					continue
				}
				logger.Infof("Created new record: %s to point to %s", name, dom.TargetDomain)
			}
			continue
		}

		for _, rec := range records {
			if rec.Content != dom.TargetDomain || c.cfg.RefreshEntries {
				if c.cfg.DryRun {
					logger.Infof("DRY-RUN: PUT to Cloudflare %s, %s: %+v", dom.ZoneID, rec.ID, data)
				} else {
					if err := c.cf.UpdateDNSRecord(dom.ZoneID, rec.ID, data); err != nil {
						logger.Errorf("%s update record failed: %v", name, err)
						ok = false
						continue
					}
					logger.Infof("Updated existing record: %s to point to %s", name, dom.TargetDomain)
				}
			} else {
				logger.Verbosef("Existing record: %s already points to %s", name, dom.TargetDomain)
			}
		}
	}
	return ok
}

func addToMappings(current, incoming map[string]int) {
	for host, source := range incoming {
		if curr, ok := current[host]; !ok || curr > source {
			current[host] = source
		}
	}
}

func parseTraefikV1HostRule(rule string) []string {
	if !strings.Contains(rule, "Host") {
		return nil
	}
	idx := strings.Index(rule, "Host:")
	if idx == -1 {
		return nil
	}
	raw := strings.TrimSpace(rule[idx+5:])
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

func parseTraefikV2Rule(rule string) []string {
	rx := regexp.MustCompile("`([a-zA-Z0-9\\.\\-]+)`")
	matches := rx.FindAllStringSubmatch(rule, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			out = append(out, m[1])
		}
	}
	return out
}

func parseTraefikRouterRule(rule string) []string {
	rx := regexp.MustCompile(`Host\(\` + "`" + `([a-zA-Z0-9\.\-]+)\` + "`" + `\)`)
	matches := rx.FindAllStringSubmatch(rule, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			out = append(out, m[1])
		}
	}
	return out
}

func isDomainExcluded(name string, dom DomainConfig) bool {
	for _, sub := range dom.ExcludedSubDomains {
		if strings.Contains(name, sub+"."+dom.Name) {
			return true
		}
	}
	return false
}

func isMatching(host string, regexes []*regexp.Regexp) bool {
	for _, re := range regexes {
		if re.MatchString(host) {
			return true
		}
	}
	return false
}

func getSecretByEnv(name string) string {
	lowerName := strings.ToLower(name)

	implicitSecretPaths := make([]string, 0, len(defaultSecretDirs)*2)
	for _, dir := range defaultSecretDirs {
		implicitSecretPaths = append(implicitSecretPaths, dir+"/"+name, dir+"/"+lowerName)
	}

	fileSpecs := []string{
		os.Getenv(name + "_FILE"),
		os.Getenv(lowerName + "_FILE"),
	}
	fileSpecs = append(fileSpecs, implicitSecretPaths...)
	for _, spec := range fileSpecs {
		if value := readSecretSpec(spec); value != "" {
			return value
		}
	}

	envCandidates := []string{
		os.Getenv(name),
		os.Getenv(lowerName),
	}
	for _, value := range envCandidates {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func readSecretSpec(spec string) string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ""
	}

	paths := []string{spec}
	if !strings.HasPrefix(spec, "/") {
		for _, dir := range defaultSecretDirs {
			paths = append(paths, dir+"/"+spec)
		}
	}

	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value := strings.TrimSpace(string(contents))
		if value != "" {
			return value
		}
	}
	return ""
}

func newDockerHTTPClient(cfg Config) (*http.Client, bool, error) {
	dockerHost := strings.TrimSpace(os.Getenv("DOCKER_HOST"))
	if dockerHost == "" {
		dockerHost = "unix:///var/run/docker.sock"
	}
	parsed, err := url.Parse(dockerHost)
	if err != nil {
		return nil, false, fmt.Errorf("invalid DOCKER_HOST: %w", err)
	}
	if parsed.Scheme != "tcp" && parsed.Scheme != "https" {
		return nil, false, nil
	}
	if strings.TrimSpace(cfg.DockerCACertFile) == "" && !cfg.DockerInsecureSkipVerify {
		return nil, false, nil
	}

	tlsCfg, err := newTLSConfig(cfg.DockerCACertFile, cfg.DockerInsecureSkipVerify)
	if err != nil {
		return nil, false, err
	}
	return &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}, true, nil
}

func newTLSConfig(caCertFile string, insecureSkipVerify bool) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: insecureSkipVerify,
	}

	caCertFile = strings.TrimSpace(caCertFile)
	if caCertFile == "" {
		return tlsCfg, nil
	}

	caPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert file %s: %w", caCertFile, err)
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil || rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}
	if ok := rootCAs.AppendCertsFromPEM(caPEM); !ok {
		return nil, fmt.Errorf("failed to parse CA cert from %s", caCertFile)
	}
	tlsCfg.RootCAs = rootCAs
	return tlsCfg, nil
}

func splitCleanCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		clean := strings.TrimSpace(p)
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

func parseBoolLikePython(raw string, defaultVal bool) bool {
	if raw == "" {
		return defaultVal
	}
	lower := strings.ToLower(raw)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}
	return defaultVal
}

func parseIntOr(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func defaultString(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func validURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

func runWithRecover(logger *Logger, scope string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("recovered panic in %s: %v", scope, r)
		}
	}()
	fn()
}
