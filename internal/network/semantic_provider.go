package network

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Operator configuration is distinct from participant credentials and intent
// content. A configured profile is not a claim of provider/model qualification.
type semanticConfig struct {
	Endpoint                string   `json:"endpoint"`
	ProviderID              string   `json:"provider_id"`
	Model                   string   `json:"model"`
	ModelRevision           string   `json:"model_revision"`
	Dimensions              int      `json:"dimensions"`
	PermissionEvidence      string   `json:"permission_evidence"`
	PrivacyPolicySHA256     string   `json:"privacy_policy_sha256"`
	AllowPublicProjections  bool     `json:"allow_public_projections"`
	AllowPrivateProjections bool     `json:"allow_private_projections"`
	MinimumSimilarity       *float64 `json:"minimum_similarity,omitempty"`
	TestOnly                bool     `json:"test_only"`
	TLSRootCAFile           string   `json:"tls_root_ca_file,omitempty"`
}
type semanticProvider struct {
	config semanticConfig
	hash   string
	client *http.Client
}

var errSemanticConfiguration = errors.New("invalid semantic provider configuration")
var errSemanticResponse = errors.New("semantic provider response unavailable or invalid")

func semanticConfiguration(environment string) (*semanticProvider, error) {
	path := os.Getenv("UNDERGRID_SEMANTIC_CONFIG_FILE")
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errSemanticConfiguration
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 32769))
	if err != nil || len(raw) > 32768 {
		return nil, errSemanticConfiguration
	}
	var config semanticConfig
	if err = decodeSemanticJSON(raw, &config); err != nil {
		return nil, errSemanticConfiguration
	}
	return newSemanticProvider(config, environment)
}
func decodeSemanticJSON(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errSemanticResponse
	}
	return nil
}
func newSemanticProvider(c semanticConfig, environment string) (*semanticProvider, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errSemanticConfiguration
	}
	for _, v := range []string{c.ProviderID, c.Model, c.ModelRevision, c.PermissionEvidence} {
		if strings.TrimSpace(v) == "" || len(v) > 1000 {
			return nil, errSemanticConfiguration
		}
	}
	policyHash, hashErr := hex.DecodeString(c.PrivacyPolicySHA256)
	if c.Dimensions < 1 || c.Dimensions > 4096 || hashErr != nil || len(policyHash) != 32 || strings.ToLower(c.PrivacyPolicySHA256) != c.PrivacyPolicySHA256 || (!c.AllowPublicProjections && !c.AllowPrivateProjections) {
		return nil, errSemanticConfiguration
	}
	threshold := 0.8
	if c.MinimumSimilarity != nil {
		threshold = *c.MinimumSimilarity
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 1 {
		return nil, errSemanticConfiguration
	}
	c.MinimumSimilarity = &threshold
	loopback := strings.EqualFold(u.Hostname(), "localhost")
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = loopback || ip.IsLoopback() || ip.IsUnspecified()
	}
	if environment == "production" && (c.TestOnly || c.TLSRootCAFile != "" || loopback) {
		return nil, errSemanticConfiguration
	}
	if (loopback || c.TLSRootCAFile != "") && !c.TestOnly {
		return nil, errSemanticConfiguration
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if c.TLSRootCAFile != "" {
		raw, err := os.ReadFile(c.TLSRootCAFile)
		if err != nil || len(raw) > 65536 {
			return nil, errSemanticConfiguration
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(raw) {
			return nil, errSemanticConfiguration
		}
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	}
	return &semanticProvider{config: c, hash: digest(canonical(c)), client: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Callers must supply a separately approved projection. Never pass a full
// intention, private objective, budget, agent credential or room content here.
// I/O belongs outside all authority-bearing database transactions.
func (p *semanticProvider) embed(ctx context.Context, projection string) ([]float32, error) {
	if len(projection) > 4000 || strings.TrimSpace(projection) == "" {
		return nil, errSemanticResponse
	}
	payload := canonical(map[string]any{"model": p.config.Model, "model_revision": p.config.ModelRevision, "input": projection})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, errSemanticResponse
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// This operator secret is neither an agent/network credential nor part of
	// the immutable public profile. No provider error body or key is surfaced.
	if key := os.Getenv("UNDERGRID_EMBEDDING_API_KEY"); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	response, err := p.client.Do(req)
	if err != nil {
		return nil, errSemanticResponse
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(raw) > 65536 || response.StatusCode != http.StatusOK {
		return nil, errSemanticResponse
	}
	var result struct {
		Model     string    `json:"model"`
		Revision  string    `json:"model_revision"`
		InputHash string    `json:"input_sha256"`
		Vector    []float64 `json:"embedding"`
	}
	if err = decodeSemanticJSON(raw, &result); err != nil || result.Model != p.config.Model || result.Revision != p.config.ModelRevision || result.InputHash != digest([]byte(projection)) || len(result.Vector) != p.config.Dimensions {
		return nil, errSemanticResponse
	}
	vector := make([]float32, len(result.Vector))
	norm := 0.0
	for i, value := range result.Vector {
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > math.MaxFloat32 {
			return nil, errSemanticResponse
		}
		vector[i] = float32(value)
		norm += float64(vector[i]) * float64(vector[i])
	}
	if norm == 0 || math.IsInf(norm, 0) {
		return nil, errSemanticResponse
	}
	return vector, nil
}
