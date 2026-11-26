package keenetic

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/imroc/req/v3"
)

type Client struct {
	baseURL    string
	login      string
	password   string
	httpClient *req.Client

	authMu        sync.Mutex
	lastAuthTime  time.Time
	authenticated bool
}

// NewClient initializes Keenetic HTTP client.
func NewClient(baseURL, login, password string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")

	c := req.C().
		SetBaseURL(baseURL).
		SetTimeout(15 * time.Second)

	return &Client{
		baseURL:    baseURL,
		login:      login,
		password:   password,
		httpClient: c,
	}
}

// Login performs challenge-response authentication with Keenetic router.
func (c *Client) Login(ctx context.Context) error {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	return c.loginLocked(ctx)
}

func (c *Client) loginLocked(ctx context.Context) error {
	// Step 1: Request /auth to receive challenge and realm
	resp, err := c.httpClient.R().
		SetContext(ctx).
		Get("/auth")
	if err != nil {
		return fmt.Errorf("initial auth challenge request failed: %w", err)
	}

	realm := resp.GetHeader("X-NDM-Realm")
	challenge := resp.GetHeader("X-NDM-Challenge")

	// If already authenticated (status 200), we are done
	if resp.StatusCode == http.StatusOK {
		c.authenticated = true
		c.lastAuthTime = time.Now()
		return nil
	}

	if realm == "" || challenge == "" {
		return fmt.Errorf("missing challenge or realm in auth response: realm=%q, challenge=%q, status=%d",
			realm, challenge, resp.StatusCode)
	}

	// Step 2: Compute MD5(login:realm:password)
	md5Raw := md5.Sum([]byte(fmt.Sprintf("%s:%s:%s", c.login, realm, c.password)))
	md5Hex := hex.EncodeToString(md5Raw[:])

	// Step 3: Compute SHA256(challenge + md5Hex)
	shaRaw := sha256.Sum256([]byte(challenge + md5Hex))
	finalPassword := hex.EncodeToString(shaRaw[:])

	// Step 4: POST /auth with login and hashed password
	authBody := map[string]string{
		"login":    c.login,
		"password": finalPassword,
	}

	authResp, err := c.httpClient.R().
		SetContext(ctx).
		SetBody(authBody).
		Post("/auth")
	if err != nil {
		return fmt.Errorf("auth credentials submission failed: %w", err)
	}

	if authResp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed with HTTP %d: %s", authResp.StatusCode, authResp.String())
	}

	c.authenticated = true
	c.lastAuthTime = time.Now()
	return nil
}

// ensureAuth guarantees client is authenticated before making requests.
func (c *Client) ensureAuth(ctx context.Context) error {
	c.authMu.Lock()
	if c.authenticated {
		c.authMu.Unlock()
		return nil
	}
	c.authMu.Unlock()

	return c.Login(ctx)
}

// execute executes an HTTP request, automatically re-authenticating if 401 is received.
func (c *Client) execute(ctx context.Context, reqFunc func() (*req.Response, error)) (*req.Response, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, err
	}

	resp, err := reqFunc()
	if err != nil {
		return nil, err
	}

	// Check if session expired
	if resp.StatusCode == http.StatusUnauthorized {
		// Token expired, re-authenticate and retry once
		if err := c.Login(ctx); err != nil {
			return nil, fmt.Errorf("re-auth after 401 failed: %w", err)
		}
		resp, err = reqFunc()
		if err != nil {
			return nil, err
		}
	}

	if resp.StatusCode >= 400 {
		return resp, fmt.Errorf("request failed with HTTP %d: %s", resp.StatusCode, resp.String())
	}

	return resp, nil
}

// GetHotspot retrieves the list of devices (hosts) on the network.
func (c *Client) GetHotspot(ctx context.Context) (*HotspotResponse, error) {
	var result HotspotResponse

	_, err := c.execute(ctx, func() (*req.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetSuccessResult(&result).
			Get("/rci/show/ip/hotspot")
	})
	if err != nil {
		return nil, fmt.Errorf("get hotspot: %w", err)
	}

	return &result, nil
}

// GetInterfaces retrieves all network interfaces.
func (c *Client) GetInterfaces(ctx context.Context) (map[string]Interface, error) {
	result := make(map[string]Interface)

	_, err := c.execute(ctx, func() (*req.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetSuccessResult(&result).
			Get("/rci/show/interface")
	})
	if err != nil {
		return nil, fmt.Errorf("get interfaces: %w", err)
	}

	return result, nil
}

// GetSystemInfo retrieves system status (CPU, memory, uptime, etc.).
func (c *Client) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	var result SystemInfo

	_, err := c.execute(ctx, func() (*req.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetSuccessResult(&result).
			Get("/rci/show/system")
	})
	if err != nil {
		return nil, fmt.Errorf("get system info: %w", err)
	}

	return &result, nil
}

// SetDeviceAccess enables or disables internet access for a host (deny / permit policy).
func (c *Client) SetDeviceAccess(ctx context.Context, mac string, permit bool) error {
	if mac == "" {
		return errors.New("mac address is required")
	}

	body := map[string]interface{}{
		"mac": mac,
	}
	if permit {
		body["permit"] = true
	} else {
		body["deny"] = true
	}

	_, err := c.execute(ctx, func() (*req.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetBody(body).
			Post("/rci/ip/hotspot/host")
	})
	if err != nil {
		return fmt.Errorf("set device access (permit=%v) for %s: %w", permit, mac, err)
	}

	return nil
}

// SetDeviceSpeedLimit sets the bandwidth limit for a host in kbits/sec.
// If rateKbps <= 0, the speed limit is removed.
func (c *Client) SetDeviceSpeedLimit(ctx context.Context, mac string, rateKbps int) error {
	if mac == "" {
		return errors.New("mac address is required")
	}

	body := map[string]interface{}{
		"mac": mac,
	}
	if rateKbps > 0 {
		body["rate"] = rateKbps
	} else {
		body["no"] = true
	}

	_, err := c.execute(ctx, func() (*req.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetBody(body).
			Post("/rci/ip/traffic-shape/host")
	})
	if err != nil {
		return fmt.Errorf("set speed limit (%d kbps) for %s: %w", rateKbps, mac, err)
	}

	return nil
}

// WakeOnLAN sends a Wake-on-LAN magic packet to a device by MAC address.
func (c *Client) WakeOnLAN(ctx context.Context, mac string) error {
	if mac == "" {
		return errors.New("mac address is required")
	}

	body := map[string]string{
		"mac": mac,
	}

	_, err := c.execute(ctx, func() (*req.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetBody(body).
			Post("/rci/ip/hotspot/wake")
	})
	if err != nil {
		return fmt.Errorf("wake on lan for %s: %w", mac, err)
	}

	return nil
}

// Reboot requests a router reboot.
func (c *Client) Reboot(ctx context.Context) error {
	_, err := c.execute(ctx, func() (*req.Response, error) {
		return c.httpClient.R().
			SetContext(ctx).
			SetBody(map[string]interface{}{}).
			Post("/rci/system/reboot")
	})
	if err != nil {
		return fmt.Errorf("reboot router: %w", err)
	}

	return nil
}
