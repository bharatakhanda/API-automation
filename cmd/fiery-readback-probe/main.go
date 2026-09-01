package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	apiV5 = "/live/api/v5"
	apiV4 = "/live/api/v4"
)

// defaultSecretKey is intentionally empty in source. It may be populated for a
// local diagnostic build with:
//
//	-ldflags "-X main.defaultSecretKey=<key>"
//
// Do not commit real secrets to source control.
var defaultSecretKey string

type config struct {
	Server      string
	JobID       string
	API         string
	Query       string
	Username    string
	Password    string
	Secret      string
	SecretsFile string
	Cookie      string
	OutDir      string
	Timeout     time.Duration
	Repeat      int
	Interval    time.Duration
	InsecureTLS bool
	Interactive bool
}

type session struct {
	Name      string `json:"name"`
	LoginPath string `json:"loginPath,omitempty"`
	Cookie    string `json:"-"`
	CookieRed string `json:"cookie,omitempty"`
}

type probeSummary struct {
	RunAt    string        `json:"runAt"`
	Server   string        `json:"server"`
	JobID    string        `json:"jobId"`
	API      string        `json:"api"`
	Query    string        `json:"query,omitempty"`
	OutDir   string        `json:"outDir"`
	Sessions []session     `json:"sessions"`
	Results  []probeResult `json:"results"`
}

type probeResult struct {
	Attempt         int               `json:"attempt"`
	Session         string            `json:"session"`
	SessionLogin    string            `json:"sessionLogin,omitempty"`
	Variant         string            `json:"variant"`
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	RequestHeaders  map[string]string `json:"requestHeaders,omitempty"`
	ResponseProto   string            `json:"responseProto,omitempty"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`
	StatusCode      int               `json:"statusCode"`
	DurationMS      int64             `json:"durationMs"`
	BodyFile        string            `json:"bodyFile,omitempty"`
	HeaderFile      string            `json:"headerFile,omitempty"`
	BodyBytes       int               `json:"bodyBytes"`
	JSON            bool              `json:"json"`
	ServerTime      string            `json:"serverTime,omitempty"`
	KeyCount        int               `json:"keyCount"`
	HasEFResolution bool              `json:"hasEFResolution"`
	EFResolution    string            `json:"EFResolution,omitempty"`
	Error           string            `json:"error,omitempty"`
}

type rawResponse struct {
	Variant         string
	URL             string
	RequestHeaders  map[string]string
	ResponseProto   string
	ResponseHeaders map[string]string
	StatusCode      int
	Duration        time.Duration
	Body            []byte
	Err             error
}

func main() {
	os.Exit(run())
}

func run() int {
	cfg := parseFlags()
	cfg = promptMissingInputs(cfg)
	if err := cfg.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr)
		flag.Usage()
		return 2
	}

	baseURL, err := normalizeServerURL(cfg.Server)
	if err != nil {
		fmt.Fprintln(os.Stderr, "server URL:", err)
		return 2
	}
	if cfg.Secret == "" && cfg.Cookie == "" {
		cfg.Secret = loadSecret(cfg.SecretsFile)
	}
	if cfg.Secret == "" {
		cfg.Secret = defaultSecretKey
	}
	if cfg.Cookie == "" && cfg.Password == "" {
		cfg.Password = os.Getenv("FIERY_PASSWORD")
	}
	if cfg.Cookie == "" && cfg.Secret == "" {
		cfg.Secret = os.Getenv("FIERY_SECRET")
	}

	if cfg.Cookie == "" && cfg.Password == "" {
		cfg.Cookie = normalizeCookieHeader(promptPassword("Existing Postman Cookie (optional; press Enter to use Fiery login): "))
		cfg.Interactive = true
	}
	if cfg.Cookie == "" && cfg.Password == "" {
		cfg.Password = promptPassword("Fiery admin password: ")
	}

	if cfg.Cookie == "" && (cfg.Password == "" || cfg.Secret == "") {
		fmt.Fprintln(os.Stderr, "provide either --cookie/ FIERY_COOKIE, or login credentials: --password plus --secret / --secrets-file / embedded secret")
		return 2
	}
	if cfg.Interactive {
		defer waitForEnter()
	}

	if cfg.Repeat < 1 {
		cfg.Repeat = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*time.Duration(maxInt(1, cfg.Repeat))+cfg.Interval*time.Duration(maxInt(0, cfg.Repeat-1))+5*time.Second)
	defer cancel()

	runDir := filepath.Join(cfg.OutDir, "readback-probe-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "create output directory:", err)
		return 1
	}

	sessions, loginErrors := buildSessions(ctx, cfg, baseURL)
	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "no usable session")
		for _, err := range loginErrors {
			fmt.Fprintln(os.Stderr, " -", err)
		}
		return 1
	}
	for i := range sessions {
		sessions[i].CookieRed = redactCookie(sessions[i].Cookie)
	}
	if strings.TrimSpace(cfg.JobID) == "" {
		latest, err := latestJobID(ctx, cfg, baseURL, sessions[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "no --job was provided and latest job lookup failed:", err)
			return 1
		}
		cfg.JobID = latest
		fmt.Println("No job ID provided; using newest visible job:", cfg.JobID)
	}

	summary := probeSummary{
		RunAt:    time.Now().Format(time.RFC3339Nano),
		Server:   baseURL,
		JobID:    cfg.JobID,
		API:      cfg.API,
		Query:    cfg.Query,
		OutDir:   runDir,
		Sessions: sessions,
	}

	apis := apiPaths(cfg.API)
	for attempt := 1; attempt <= cfg.Repeat; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				fmt.Fprintln(os.Stderr, ctx.Err())
				return 1
			case <-time.After(cfg.Interval):
			}
		}
		for _, apiPath := range apis {
			endpoint := baseURL + apiPath + "/jobs/" + url.PathEscape(cfg.JobID)
			if strings.TrimSpace(cfg.Query) != "" {
				endpoint += "?" + strings.TrimPrefix(strings.TrimSpace(cfg.Query), "?")
			}
			for _, sess := range sessions {
				for _, raw := range runVariants(ctx, cfg, sess, endpoint) {
					result := summarizeRawResponse(attempt, sess, raw, runDir, len(summary.Results)+1)
					summary.Results = append(summary.Results, result)
					printOneLine(result)
				}
			}
		}
	}

	summaryPath := filepath.Join(runDir, "summary.json")
	if err := writeJSON(summaryPath, summary); err != nil {
		fmt.Fprintln(os.Stderr, "write summary:", err)
		return 1
	}
	fmt.Println()
	fmt.Println("Saved summary:", summaryPath)
	fmt.Println("Saved raw response bodies under:", runDir)
	return 0
}

func parseFlags() config {
	cfg := config{
		API:         "v5",
		Username:    "admin",
		SecretsFile: ".local/secrets.json",
		Cookie:      os.Getenv("FIERY_COOKIE"),
		OutDir:      defaultCaptureDir(),
		Timeout:     30 * time.Second,
		Repeat:      1,
		Interval:    5 * time.Second,
		InsecureTLS: true,
	}
	flag.StringVar(&cfg.Server, "server", cfg.Server, "Fiery server IP or URL, e.g. 10.220.129.85")
	flag.StringVar(&cfg.JobID, "job", cfg.JobID, "Fiery job ID to GET")
	flag.StringVar(&cfg.API, "api", cfg.API, "readback API: v5, v4, or both")
	flag.StringVar(&cfg.Query, "query", cfg.Query, "optional raw query string to append to the job GET")
	flag.StringVar(&cfg.Username, "username", cfg.Username, "Fiery username for login mode")
	flag.StringVar(&cfg.Password, "password", cfg.Password, "Fiery password for login mode; alternatively set FIERY_PASSWORD")
	flag.StringVar(&cfg.Secret, "secret", cfg.Secret, "Fiery API key; alternatively set FIERY_SECRET or use --secrets-file")
	flag.StringVar(&cfg.SecretsFile, "secrets-file", cfg.SecretsFile, "JSON file containing secretKey, key, apikey, or accessrights")
	flag.StringVar(&cfg.Cookie, "cookie", cfg.Cookie, "existing Cookie header, e.g. _session_id=...; alternatively set FIERY_COOKIE")
	flag.StringVar(&cfg.OutDir, "out-dir", cfg.OutDir, "directory for summary and raw response bodies")
	flag.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "per-request timeout")
	flag.IntVar(&cfg.Repeat, "repeat", cfg.Repeat, "number of probe rounds")
	flag.DurationVar(&cfg.Interval, "interval", cfg.Interval, "delay between repeated probe rounds")
	flag.BoolVar(&cfg.InsecureTLS, "insecure", cfg.InsecureTLS, "skip TLS certificate verification")
	flag.Parse()
	return cfg
}

func promptMissingInputs(cfg config) config {
	if strings.TrimSpace(cfg.Server) == "" {
		cfg.Server = readLine("Fiery server IP or URL: ")
		cfg.Interactive = true
	}
	if strings.TrimSpace(cfg.JobID) == "" {
		cfg.JobID = readLine("Fiery job ID (optional; press Enter to use newest visible job): ")
		cfg.Interactive = true
	}
	return cfg
}

func readLine(prompt string) string {
	fmt.Print(prompt)
	text, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(text)
}

func promptPassword(prompt string) string {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return readLine(prompt)
	}
	return strings.TrimSpace(string(password))
}

func normalizeCookieHeader(cookie string) string {
	cookie = strings.TrimSpace(cookie)
	if name, value, ok := strings.Cut(cookie, ":"); ok && strings.EqualFold(strings.TrimSpace(name), "Cookie") {
		cookie = strings.TrimSpace(value)
	}
	if cookie != "" && !strings.Contains(cookie, "=") {
		cookie = "_session_id=" + cookie
	}
	return cookie
}

func waitForEnter() {
	fmt.Println()
	fmt.Print("Press Enter to exit...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func (cfg config) validate() error {
	if strings.TrimSpace(cfg.Server) == "" {
		return errors.New("--server is required")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.API)) {
	case "v5", "v4", "both":
		return nil
	default:
		return errors.New("--api must be v5, v4, or both")
	}
}

func defaultCaptureDir() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, "Downloads", "captures")
	}
	return "captures"
}

func normalizeServerURL(server string) (string, error) {
	server = strings.TrimSpace(server)
	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "https://" + server
	}
	parsed, err := url.Parse(server)
	if err != nil {
		return "", err
	}
	if parsed.Host == "" {
		return "", errors.New("missing host")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func latestJobID(ctx context.Context, cfg config, baseURL string, sess session) (string, error) {
	var failures []string
	for _, apiPath := range apiPaths(cfg.API) {
		endpoint := baseURL + apiPath + "/jobs"
		raw := goJobGET(ctx, cfg, endpoint, "latest-job-list", map[string]string{"Cookie": sess.Cookie, "Accept": "*/*"}, true, true, true)
		if raw.Err != nil {
			failures = append(failures, fmt.Sprintf("%s error: %v", apiPath, raw.Err))
			continue
		}
		if raw.StatusCode < 200 || raw.StatusCode >= 300 {
			failures = append(failures, fmt.Sprintf("%s HTTP %d", apiPath, raw.StatusCode))
			continue
		}
		jobID, err := latestJobIDFromList(raw.Body)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s parse: %v", apiPath, err))
			continue
		}
		return jobID, nil
	}
	return "", errors.New(strings.Join(failures, "; "))
}

func latestJobIDFromList(body []byte) (string, error) {
	var payload struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if len(payload.Data.Items) == 0 {
		return "", errors.New("job list is empty")
	}
	bestID := ""
	bestScore := ""
	for idx, item := range payload.Data.Items {
		id := cleanString(item["id"])
		if id == "" {
			continue
		}
		score := jobListScore(item, idx)
		if bestID == "" || score > bestScore {
			bestID = id
			bestScore = score
		}
	}
	if bestID == "" {
		return "", errors.New("no item.id was present in job list")
	}
	return bestID, nil
}

func jobListScore(item map[string]any, idx int) string {
	for _, key := range []string{"timestamp created", "timestamp touched", "time", "creation time", "job number"} {
		if value := cleanString(item[key]); value != "" {
			return value
		}
	}
	return fmt.Sprintf("%020d", idx)
}

func cleanString(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "<nil>" {
		return ""
	}
	return text
}

func apiPaths(api string) []string {
	switch strings.ToLower(strings.TrimSpace(api)) {
	case "v4":
		return []string{apiV4}
	case "both":
		return []string{apiV5, apiV4}
	default:
		return []string{apiV5}
	}
}

func loadSecret(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var payload map[string]string
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	for _, key := range []string{"secretKey", "key", "apikey", "accessrights"} {
		if value := strings.TrimSpace(payload[key]); value != "" {
			return value
		}
	}
	return ""
}

func buildSessions(ctx context.Context, cfg config, baseURL string) ([]session, []error) {
	if cookie := strings.TrimSpace(cfg.Cookie); cookie != "" {
		return []session{{Name: "provided-cookie", LoginPath: "provided", Cookie: cookie}}, nil
	}
	var sessions []session
	var errs []error
	for _, apiPath := range []string{apiV5, apiV4} {
		name := "go-login-" + strings.TrimPrefix(apiPath, "/live/api/")
		cookie, err := goLogin(ctx, cfg, baseURL, apiPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		} else {
			sessions = append(sessions, session{Name: name, LoginPath: apiPath, Cookie: cookie})
		}
	}
	if _, err := curlExecutable(); err != nil {
		errs = append(errs, err)
		return sessions, errs
	}
	for _, apiPath := range []string{apiV5, apiV4} {
		name := "curl-login-" + strings.TrimPrefix(apiPath, "/live/api/")
		cookie, err := curlLogin(ctx, cfg, baseURL, apiPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		} else {
			sessions = append(sessions, session{Name: name, LoginPath: "curl " + apiPath, Cookie: cookie})
		}
	}
	return sessions, errs
}

func loginPayload(cfg config, apiPath string) map[string]string {
	payload := map[string]string{
		"username": cfg.Username,
		"password": cfg.Password,
	}
	if apiPath == apiV5 {
		payload["apikey"] = cfg.Secret
	} else {
		payload["accessrights"] = cfg.Secret
	}
	return payload
}

func goLogin(ctx context.Context, cfg config, baseURL, apiPath string) (string, error) {
	payload, err := json.Marshal(loginPayload(cfg, apiPath))
	if err != nil {
		return "", err
	}
	client := &http.Client{Transport: transport(cfg.InsecureTLS, true, true), Timeout: cfg.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+apiPath+"/login", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "PostmanRuntime/7.51.1")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return "", errors.New("login succeeded but no Set-Cookie was returned")
	}
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; "), nil
}

func curlLogin(ctx context.Context, cfg config, baseURL, apiPath string) (string, error) {
	payload, err := json.Marshal(loginPayload(cfg, apiPath))
	if err != nil {
		return "", err
	}
	curl, err := curlExecutable()
	if err != nil {
		return "", err
	}
	headers, err := os.CreateTemp("", "fiery-readback-probe-login-*.headers")
	if err != nil {
		return "", err
	}
	headerPath := headers.Name()
	_ = headers.Close()
	defer os.Remove(headerPath)

	const marker = "\n__FIERY_READBACK_PROBE_STATUS__:"
	args := []string{"--location", "--insecure", "--http1.1", "--silent", "--show-error", "--max-time", secondsString(cfg.Timeout), "--dump-header", headerPath, "--output", "-", "--write-out", marker + "%{http_code}", "--header", "Content-Type: application/json", "--header", "Accept: */*", "--header", "User-Agent: PostmanRuntime/7.51.1", "--header", "Postman-Token: fiery-readback-probe-login", "--data-binary", "@-", baseURL + apiPath + "/login"}
	cmd := exec.CommandContext(ctx, curl, args...)
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	text := string(output)
	status := 0
	if idx := strings.LastIndex(text, marker); idx >= 0 {
		_, _ = fmt.Sscanf(strings.TrimSpace(text[idx+len(marker):]), "%d", &status)
	}
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("HTTP %d", status)
	}
	headerBytes, err := os.ReadFile(headerPath)
	if err != nil {
		return "", err
	}
	cookie := cookieHeaderFromSetCookie(string(headerBytes))
	if cookie == "" {
		return "", errors.New("login succeeded but no Set-Cookie was returned")
	}
	return cookie, nil
}

func runVariants(ctx context.Context, cfg config, sess session, endpoint string) []rawResponse {
	cookieHeader := map[string]string{"Cookie": sess.Cookie}
	postmanHeaders := map[string]string{
		"Cookie":          sess.Cookie,
		"Postman-Token":   "fiery-readback-probe",
		"User-Agent":      "PostmanRuntime/7.51.1",
		"Accept":          "*/*",
		"Accept-Encoding": "gzip, deflate, br",
		"Connection":      "keep-alive",
	}
	return []rawResponse{
		goJobGET(ctx, cfg, endpoint, "go-default-cookie", cookieHeader, false, false, false),
		goJobGET(ctx, cfg, endpoint, "go-h1-cookie-only", cookieHeader, true, true, true),
		goJobGET(ctx, cfg, endpoint, "go-h1-postman-visible", postmanHeaders, true, true, false),
		curlJobGET(ctx, cfg, endpoint, "curl-cookie-only", cookieHeader, []string{"--location", "--insecure", "--http1.1", "--silent", "--show-error"}),
		curlJobGET(ctx, cfg, endpoint, "curl-postman-visible", postmanHeaders, []string{"--location", "--insecure", "--http1.1", "--silent", "--show-error"}),
	}
}

func goJobGET(ctx context.Context, cfg config, endpoint, variant string, headers map[string]string, forceH1, disableCompression, suppressUserAgent bool) rawResponse {
	started := time.Now()
	result := rawResponse{Variant: variant, URL: endpoint, RequestHeaders: redactedHeaderSnapshot(headers)}
	client := &http.Client{Transport: transport(cfg.InsecureTLS, forceH1, disableCompression), Timeout: cfg.Timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		result.Err = err
		return result
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if suppressUserAgent {
		req.Header["User-Agent"] = nil
	}
	resp, err := client.Do(req)
	result.Duration = time.Since(started)
	if err != nil {
		result.Err = err
		return result
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if readErr != nil {
		result.Err = readErr
	}
	if !resp.Uncompressed {
		decoded, decodeErr := decodeHTTPBody(body, resp.Header.Get("Content-Encoding"))
		if decodeErr == nil {
			body = decoded
		} else if result.Err == nil {
			result.Err = decodeErr
		}
	}
	result.ResponseProto = resp.Proto
	result.ResponseHeaders = redactHeaderSnapshot(headerSnapshot(resp.Header))
	result.StatusCode = resp.StatusCode
	result.Body = body
	return result
}

func curlJobGET(ctx context.Context, cfg config, endpoint, variant string, headers map[string]string, options []string) rawResponse {
	started := time.Now()
	result := rawResponse{Variant: variant, URL: endpoint, RequestHeaders: redactedHeaderSnapshot(headers), ResponseProto: "curl --http1.1"}
	curl, err := curlExecutable()
	if err != nil {
		result.Err = err
		return result
	}
	headerFile, err := os.CreateTemp("", "fiery-readback-probe-get-*.headers")
	if err != nil {
		result.Err = err
		return result
	}
	headerPath := headerFile.Name()
	_ = headerFile.Close()
	defer os.Remove(headerPath)

	const marker = "\n__FIERY_READBACK_PROBE_STATUS__:"
	args := append([]string{}, options...)
	args = append(args, "--max-time", secondsString(cfg.Timeout), "--dump-header", headerPath, "--write-out", marker+"%{http_code}", "--config", "-", endpoint)
	cmd := exec.CommandContext(ctx, curl, args...)
	cmd.Stdin = strings.NewReader(curlConfigHeaders(headers))
	output, err := cmd.CombinedOutput()
	result.Duration = time.Since(started)
	markerBytes := []byte(marker)
	if idx := bytes.LastIndex(output, markerBytes); idx >= 0 {
		result.Body = output[:idx]
		_, _ = fmt.Sscanf(strings.TrimSpace(string(output[idx+len(markerBytes):])), "%d", &result.StatusCode)
	} else {
		result.Body = output
	}
	if headerBytes, readErr := os.ReadFile(headerPath); readErr == nil {
		result.ResponseHeaders = redactHeaderSnapshot(parseCurlResponseHeaders(string(headerBytes)))
	}
	if encoding := headerValue(result.ResponseHeaders, "Content-Encoding"); strings.TrimSpace(encoding) != "" {
		decoded, decodeErr := decodeHTTPBody(result.Body, encoding)
		if decodeErr == nil {
			result.Body = decoded
		} else if err == nil {
			result.Err = decodeErr
		}
	}
	if err != nil {
		result.Err = err
	}
	return result
}

func transport(insecure, forceH1, disableCompression bool) http.RoundTripper {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableCompression = disableCompression
	if forceH1 {
		tr.ForceAttemptHTTP2 = false
		tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return tr
}

func summarizeRawResponse(attempt int, sess session, raw rawResponse, runDir string, index int) probeResult {
	bodyFile, headerFile := writeRawArtifacts(runDir, index, sess.Name, raw)
	serverTime, keyCount, hasEF, efValue, isJSON := inspectJobBody(raw.Body)
	result := probeResult{
		Attempt:         attempt,
		Session:         sess.Name,
		SessionLogin:    sess.LoginPath,
		Variant:         raw.Variant,
		Method:          http.MethodGet,
		URL:             raw.URL,
		RequestHeaders:  raw.RequestHeaders,
		ResponseProto:   raw.ResponseProto,
		ResponseHeaders: raw.ResponseHeaders,
		StatusCode:      raw.StatusCode,
		DurationMS:      raw.Duration.Milliseconds(),
		BodyFile:        bodyFile,
		HeaderFile:      headerFile,
		BodyBytes:       len(raw.Body),
		JSON:            isJSON,
		ServerTime:      serverTime,
		KeyCount:        keyCount,
		HasEFResolution: hasEF,
		EFResolution:    efValue,
	}
	if raw.Err != nil {
		result.Error = raw.Err.Error()
	}
	return result
}

func writeRawArtifacts(runDir string, index int, sessionName string, raw rawResponse) (string, string) {
	stem := fmt.Sprintf("%02d-%s-%s", index, sanitizeFileName(sessionName), sanitizeFileName(raw.Variant))
	bodyRel := stem + ".body"
	if json.Valid(raw.Body) {
		var pretty bytes.Buffer
		if json.Indent(&pretty, raw.Body, "", "  ") == nil {
			bodyRel = stem + ".json"
			_ = os.WriteFile(filepath.Join(runDir, bodyRel), pretty.Bytes(), 0o600)
		} else {
			_ = os.WriteFile(filepath.Join(runDir, bodyRel), raw.Body, 0o600)
		}
	} else {
		_ = os.WriteFile(filepath.Join(runDir, bodyRel), raw.Body, 0o600)
	}
	headerRel := stem + ".headers.json"
	_ = writeJSON(filepath.Join(runDir, headerRel), raw.ResponseHeaders)
	return bodyRel, headerRel
}

func printOneLine(result probeResult) {
	ef := "-"
	if result.HasEFResolution {
		ef = result.EFResolution
	}
	err := ""
	if result.Error != "" {
		err = " err=" + result.Error
	}
	fmt.Printf("attempt=%d session=%s variant=%s status=%d proto=%s bytes=%d keys=%d EFResolution=%s time=%s%s\n",
		result.Attempt, result.Session, result.Variant, result.StatusCode, result.ResponseProto, result.BodyBytes, result.KeyCount, ef, result.ServerTime, err)
}

func inspectJobBody(body []byte) (serverTime string, keyCount int, hasEF bool, efValue string, isJSON bool) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return "", 0, false, "", false
	}
	isJSON = true
	serverTime, _ = payload["time"].(string)
	item := dataItem(payload)
	if len(item) == 0 {
		return serverTime, 0, false, "", true
	}
	keyCount = len(item)
	if value, ok := item["EFResolution"]; ok {
		hasEF = true
		efValue = strings.TrimSpace(fmt.Sprint(value))
	}
	return serverTime, keyCount, hasEF, efValue, true
}

func dataItem(payload map[string]any) map[string]any {
	data, _ := payload["data"].(map[string]any)
	if item, ok := data["item"].(map[string]any); ok {
		return item
	}
	if items, ok := data["items"].([]any); ok && len(items) > 0 {
		if item, ok := items[0].(map[string]any); ok {
			return item
		}
	}
	return nil
}

func decodeHTTPBody(body []byte, encoding string) ([]byte, error) {
	encoding = strings.ToLower(strings.TrimSpace(strings.Split(encoding, ",")[0]))
	switch encoding {
	case "", "identity":
		return body, nil
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return body, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	case "deflate":
		if reader, err := zlib.NewReader(bytes.NewReader(body)); err == nil {
			defer reader.Close()
			return io.ReadAll(reader)
		}
		reader := flate.NewReader(bytes.NewReader(body))
		defer reader.Close()
		return io.ReadAll(reader)
	case "br":
		return body, errors.New("brotli content-encoding is not supported by this standard-library probe")
	default:
		return body, fmt.Errorf("unsupported content-encoding %q", encoding)
	}
}

func cookieHeaderFromSetCookie(headers string) string {
	var cookies []string
	for _, line := range strings.Split(strings.ReplaceAll(headers, "\r\n", "\n"), "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || !strings.EqualFold(name, "set-cookie") {
			continue
		}
		cookie := strings.TrimSpace(strings.SplitN(value, ";", 2)[0])
		if cookie != "" {
			cookies = append(cookies, cookie)
		}
	}
	return strings.Join(cookies, "; ")
}

func curlExecutable() (string, error) {
	for _, name := range []string{"curl.exe", "curl"} {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", errors.New("curl executable not found")
}

func curlConfigHeaders(headers map[string]string) string {
	keys := []string{"Cookie", "Postman-Token", "User-Agent", "Accept", "Accept-Encoding", "Connection"}
	var b strings.Builder
	for _, key := range keys {
		value := headers[key]
		if strings.TrimSpace(value) == "" {
			continue
		}
		b.WriteString("header = \"")
		b.WriteString(escapeCurlConfig(key + ": " + value))
		b.WriteString("\"\n")
	}
	return b.String()
}

func escapeCurlConfig(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}

func parseCurlResponseHeaders(raw string) map[string]string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	blocks := strings.Split(raw, "\n\n")
	for i := len(blocks) - 1; i >= 0; i-- {
		block := strings.TrimSpace(blocks[i])
		if block == "" || !strings.HasPrefix(strings.ToUpper(block), "HTTP/") {
			continue
		}
		lines := strings.Split(block, "\n")
		out := map[string]string{"Status-Line": strings.TrimSpace(lines[0])}
		for _, line := range lines[1:] {
			name, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			value = strings.TrimSpace(value)
			if name == "" {
				continue
			}
			if existing := out[name]; existing != "" {
				out[name] = existing + ", " + value
			} else {
				out[name] = value
			}
		}
		return out
	}
	return nil
}

func headerValue(headers map[string]string, want string) string {
	for key, value := range headers {
		if strings.EqualFold(key, want) {
			return value
		}
	}
	return ""
}

func headerSnapshot(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		out[key] = strings.Join(values, ", ")
	}
	return out
}

func redactedHeaderSnapshot(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		if strings.EqualFold(key, "Cookie") {
			out[key] = redactCookie(value)
		} else {
			out[key] = value
		}
	}
	return out
}

func redactHeaderSnapshot(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		if strings.EqualFold(key, "Set-Cookie") {
			out[key] = redactCookie(value)
		} else {
			out[key] = value
		}
	}
	return out
}

func redactCookie(cookie string) string {
	parts := strings.Split(cookie, ";")
	for i, part := range parts {
		name, _, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name != "" {
			parts[i] = name + "=<redacted>"
		}
	}
	return strings.Join(parts, "; ")
}

func sanitizeFileName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "empty"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o600)
}

func secondsString(duration time.Duration) string {
	seconds := int(duration.Round(time.Second).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprint(seconds)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
