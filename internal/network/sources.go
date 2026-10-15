package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// SourceConfig is trusted operator configuration, never agent supplied. Basis
// strings record external approval evidence; configuring them does not grant it.
type SourceConfig struct {
	ID, Kind, Endpoint, Publisher, AccessBasis, RepublicationBasis, TombstoneBasis string
	Fields, Labels, Takedowns                                                      []string
}
type SourceRefresher struct {
	dsn     string
	configs []SourceConfig
	client  *http.Client
}

func publicSourceIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	for _, prefix := range []string{"0.0.0.0/8", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(prefix).Contains(a) {
			return false
		}
	}
	return a.IsGlobalUnicast() && !a.IsPrivate() && !a.IsLoopback() && !a.IsLinkLocalUnicast() && !a.IsUnspecified() && !netip.MustParsePrefix("100.64.0.0/10").Contains(a)
}
func NewSourceRefresher(dsn string, configs []SourceConfig) (*SourceRefresher, error) {
	seen := map[string]bool{}
	for _, c := range configs {
		u, e := url.Parse(c.Endpoint)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || (u.Port() != "" && u.Port() != "443") || !validID(c.ID) || seen[c.ID] || c.Publisher == "" || c.AccessBasis == "" || c.RepublicationBasis == "" || c.TombstoneBasis == "" || len(c.Fields) == 0 {
			return nil, errors.New("invalid approved source configuration")
		}
		if ip := net.ParseIP(u.Hostname()); ip != nil && !publicSourceIP(ip) {
			return nil, errors.New("nonpublic source endpoint")
		}
		if c.Kind != "capability_manifest" && c.Kind != "github_requests" {
			return nil, errors.New("unsupported source kind")
		}
		if c.Kind == "github_requests" && (u.Hostname() != "api.github.com" || len(c.Labels) == 0 || !strings.HasPrefix(u.Path, "/repos/") || !strings.HasSuffix(u.Path, "/issues")) {
			return nil, errors.New("invalid opted-in repository")
		}
		if c.Kind == "github_requests" {
			parts := strings.Split(u.Path, "/")
			if len(parts) != 5 || parts[0] != "" || parts[1] != "repos" || parts[2] == "" || parts[3] == "" || parts[4] != "issues" || parts[2] == ".." || parts[3] == ".." {
				return nil, errors.New("invalid repository issue endpoint")
			}
		}
		for _, f := range c.Fields {
			if f != "canonical_url" && f != "title" && f != "summary" && f != "tags" && f != "updated_at" {
				return nil, errors.New("unapproved metadata field")
			}
		}
		seen[c.ID] = true
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, errors.New("empty source DNS")
		}
		for _, ip := range ips {
			if !publicSourceIP(ip) {
				return nil, errors.New("nonpublic source DNS")
			}
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	return &SourceRefresher{dsn: dsn, configs: configs, client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func sourceConfiguration(dsn string) (*SourceRefresher, error) {
	path := os.Getenv("UNDERGRID_SOURCES_FILE")
	if path == "" {
		return NewSourceRefresher(dsn, nil)
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	var configs []SourceConfig
	d := json.NewDecoder(io.LimitReader(f, 128*1024))
	d.DisallowUnknownFields()
	if e = d.Decode(&configs); e != nil {
		return nil, e
	}
	if len(configs) > 32 {
		return nil, errors.New("too many sources")
	}
	return NewSourceRefresher(dsn, configs)
}
func (r *SourceRefresher) RefreshDue(ctx context.Context) error {
	db, e := pgx.Connect(ctx, r.dsn)
	if e != nil {
		return e
	}
	defer db.Close(ctx)
	if _, e = db.Exec(ctx, "SET ROLE undergrid_worker"); e != nil {
		return e
	}
	// The worker file is the complete approved source set. Removing a source
	// disables discovery immediately, rather than extending its old approval.
	ids := make([]string, 0, len(r.configs))
	for _, c := range r.configs {
		ids = append(ids, c.ID)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE undergrid.external_sources SET last_success=NULL,next_refresh=now(),validators='{}' WHERE NOT(id=ANY($1::uuid[]))`, ids); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE undergrid.external_observations SET status='taken_down',metadata='{}' WHERE NOT(source_id=ANY($1::uuid[]))`, ids); err != nil {
		return err
	}
	for _, c := range r.configs {
		raw, _ := json.Marshal(c)
		if _, err = tx.Exec(ctx, `UPDATE undergrid.external_observations SET status='taken_down',metadata='{}' WHERE source_id=$1 AND EXISTS(SELECT 1 FROM undergrid.external_sources WHERE id=$1 AND config<>$2::jsonb)`, c.ID, raw); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO undergrid.external_sources(id,config) VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET config=excluded.config,validators=CASE WHEN external_sources.config<>excluded.config THEN '{}'::jsonb ELSE external_sources.validators END,last_success=CASE WHEN external_sources.config<>excluded.config THEN NULL ELSE external_sources.last_success END,next_refresh=CASE WHEN external_sources.config<>excluded.config THEN now() ELSE external_sources.next_refresh END`, c.ID, raw); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	var first error
	for _, c := range r.configs {
		if e = r.refresh(ctx, db, c); e != nil {
			delay := time.Minute
			var rate *sourceHTTPError
			if errors.As(e, &rate) && rate.retry > delay {
				delay = rate.retry
			}
			if delay > 24*time.Hour {
				delay = 24 * time.Hour
			}
			if _, scheduleErr := db.Exec(ctx, `UPDATE undergrid.external_sources SET next_refresh=now()+($2::bigint*interval '1 second'),last_error='refresh_failed' WHERE id=$1`, c.ID, int64(delay/time.Second)); scheduleErr != nil && first == nil {
				first = scheduleErr
			}
			if first == nil {
				first = e
			}
		}
	}
	return first
}

type sourceItem struct {
	key, version, canonical, status string
	metadata                        map[string]any
}

func (r *SourceRefresher) refresh(ctx context.Context, db *pgx.Conn, c SourceConfig) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var due bool
	var validatorsRaw []byte
	if e = tx.QueryRow(ctx, `SELECT next_refresh<=now(),validators FROM undergrid.external_sources WHERE id=$1 FOR UPDATE`, c.ID).Scan(&due, &validatorsRaw); e != nil {
		return e
	}
	if !due {
		return tx.Commit(ctx)
	}
	// Per-source row lock serializes processes. Bounded I/O prevents unbounded locks;
	// failed pages roll back the whole snapshot, retaining the previous good check.
	validators := map[string]string{}
	json.Unmarshal(validatorsRaw, &validators)
	nextValidators := map[string]string{}
	items := []sourceItem{}
	unchanged := false
	for page := 1; page <= 10; page++ {
		u, _ := url.Parse(c.Endpoint)
		q := u.Query()
		if c.Kind == "github_requests" {
			q.Set("state", "all")
			q.Set("labels", strings.Join(c.Labels, ","))
			q.Set("per_page", "100")
			q.Set("page", strconv.Itoa(page))
		} else if page > 1 {
			q.Set("page", strconv.Itoa(page))
		}
		u.RawQuery = q.Encode()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Undergrid-permissioned-source/0.1")
		req.Header.Set("If-None-Match", validators[strconv.Itoa(page)])
		resp, err := r.client.Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
		resp.Body.Close()
		if err != nil {
			return err
		}
		if len(body) > 1024*1024 {
			return errors.New("source page exceeds budget")
		}
		if resp.StatusCode == 304 && page == 1 && validators["1"] != "" {
			unchanged = true
			nextValidators = validators
			break
		}
		if resp.StatusCode != 200 {
			delay := time.Minute
			if seconds, err := strconv.ParseInt(resp.Header.Get("Retry-After"), 10, 64); err == nil && seconds > 0 && seconds <= 86400 {
				delay = time.Duration(seconds) * time.Second
			} else if when, err := http.ParseTime(resp.Header.Get("Retry-After")); err == nil && time.Until(when) > delay {
				delay = time.Until(when)
			}
			if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil && resp.Header.Get("X-RateLimit-Remaining") == "0" && time.Until(time.Unix(reset, 0)) > delay {
				delay = time.Until(time.Unix(reset, 0))
			}
			return &sourceHTTPError{status: resp.StatusCode, retry: delay}
		}
		nextValidators[strconv.Itoa(page)] = resp.Header.Get("ETag")
		parsed, more, err := parseSourcePage(c, body, page)
		if err != nil {
			return err
		}
		for _, newItem := range parsed {
			for _, oldItem := range items {
				if oldItem.key == newItem.key {
					return errors.New("duplicate source key across pages")
				}
			}
		}
		items = append(items, parsed...)
		if len(items) > 1000 {
			return errors.New("source item budget exceeded")
		}
		if !more {
			break
		}
		if page == 10 {
			return errors.New("source pagination budget exhausted")
		}
	}
	if unchanged {
		_, e = tx.Exec(ctx, `UPDATE undergrid.external_observations SET checked_at=now() WHERE source_id=$1 AND status IN ('active','closed')`, c.ID)
	} else {
		_, e = tx.Exec(ctx, `UPDATE undergrid.external_observations SET status='deleted',metadata='{}' WHERE source_id=$1`, c.ID)
		for _, item := range items {
			if e != nil {
				return e
			}
			b, _ := json.Marshal(item.metadata)
			h := sha256.Sum256(b)
			kind := "external_capability"
			if c.Kind == "github_requests" {
				kind = "external_request"
			}
			_, e = tx.Exec(ctx, `INSERT INTO undergrid.external_observations(id,source_id,source_key,source_version,canonical_url,kind,metadata,content_hash,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(source_id,source_key,source_version) DO UPDATE SET source_version=excluded.source_version,canonical_url=excluded.canonical_url,metadata=excluded.metadata,content_hash=excluded.content_hash,status=excluded.status,last_seen=now(),checked_at=now()`, NewID(), c.ID, item.key, item.version, item.canonical, kind, b, hex.EncodeToString(h[:]), item.status)
		}
	}
	if e != nil {
		return e
	}
	for _, key := range c.Takedowns {
		if _, e = tx.Exec(ctx, `UPDATE undergrid.external_observations SET status='taken_down',metadata='{}' WHERE source_id=$1 AND source_key=$2`, c.ID, key); e != nil {
			return e
		}
	}
	// A first-page validator cannot prove later pages unchanged. Only retain
	// conditional validators for single-page snapshots.
	if len(nextValidators) > 1 {
		nextValidators = map[string]string{}
	}
	v, _ := json.Marshal(nextValidators)
	if _, e = tx.Exec(ctx, `UPDATE undergrid.external_sources SET validators=$2,last_success=now(),next_refresh=now()+interval '6 hours',last_error=NULL WHERE id=$1`, c.ID, v); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func parseSourcePage(c SourceConfig, body []byte, page int) ([]sourceItem, bool, error) {
	var raw []map[string]json.RawMessage
	more := false
	if c.Kind == "github_requests" {
		if e := json.Unmarshal(body, &raw); e != nil {
			return nil, false, e
		}
		more = len(raw) == 100
	} else {
		var envelope struct {
			Items []map[string]json.RawMessage `json:"items"`
			Next  int                          `json:"next_page"`
		}
		if e := json.Unmarshal(body, &envelope); e != nil {
			return nil, false, e
		}
		raw = envelope.Items
		if envelope.Next != 0 && envelope.Next != page+1 {
			return nil, false, errors.New("invalid source next page")
		}
		more = envelope.Next != 0
	}
	if raw == nil {
		return nil, false, errors.New("source page must contain an items array")
	}
	if len(raw) > 100 {
		return nil, false, errors.New("source page item budget exceeded")
	}
	items := []sourceItem{}
	seen := map[string]bool{}
	for _, m := range raw {
		str := func(key string) string { var s string; json.Unmarshal(m[key], &s); return s }
		key, canonical, version, status := str("id"), str("url"), str("version"), str("status")
		if c.Kind == "github_requests" {
			if _, pr := m["pull_request"]; pr {
				continue
			}
			key = string(m["id"])
			if value, err := strconv.ParseUint(key, 10, 64); err != nil || value == 0 {
				return nil, false, errors.New("invalid GitHub issue ID")
			}
			canonical = str("html_url")
			version = str("updated_at")
			status = str("state")
			var labels []struct {
				Name string `json:"name"`
			}
			if e := json.Unmarshal(m["labels"], &labels); e != nil {
				return nil, false, e
			}
			ok := false
			for _, l := range labels {
				for _, wanted := range c.Labels {
					if l.Name == wanted {
						ok = true
					}
				}
			}
			if !ok {
				continue
			}
		}
		u, e := url.Parse(canonical)
		if key == "" || len(key) > 256 || version == "" || e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || seen[key] {
			return nil, false, errors.New("invalid or duplicate source identity")
		}
		if c.Kind == "github_requests" {
			endpoint, _ := url.Parse(c.Endpoint)
			repo := strings.TrimSuffix(strings.TrimPrefix(endpoint.Path, "/repos/"), "/issues")
			if u.Host != "github.com" || !strings.HasPrefix(u.Path, "/"+repo+"/issues/") {
				return nil, false, errors.New("issue link outside approved repository")
			}
		}
		seen[key] = true
		state := "active"
		if status == "closed" || status == "withdrawn" {
			state = "closed"
		} else if status != "active" && status != "open" {
			return nil, false, errors.New("invalid source status")
		}
		metadata := map[string]any{}
		if state == "active" {
			for _, field := range c.Fields {
				if field == "canonical_url" {
					metadata[field] = canonical
					continue
				}
				if field == "tags" {
					var tags []string
					if c.Kind == "github_requests" {
						continue
					}
					if len(m[field]) > 0 {
						if e = json.Unmarshal(m[field], &tags); e != nil || len(tags) > 30 {
							return nil, false, errors.New("invalid source tags")
						}
						for _, tag := range tags {
							if len(tag) > 100 {
								return nil, false, errors.New("source tag exceeds budget")
							}
						}
						metadata[field] = tags
					}
					continue
				}
				value := str(field)
				if len(value) > 2000 {
					return nil, false, errors.New("source field exceeds budget")
				}
				if value != "" {
					metadata[field] = value
				}
			}
		}
		items = append(items, sourceItem{key, version, canonical, state, metadata})
	}
	return items, more, nil
}

type sourceHTTPError struct {
	status int
	retry  time.Duration
}

func (e *sourceHTTPError) Error() string { return fmt.Sprintf("source HTTP status %d", e.status) }
