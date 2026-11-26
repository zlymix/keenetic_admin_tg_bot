package keenetic

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// HotspotResponse represents response from /rci/show/ip/hotspot
type HotspotResponse struct {
	Host []Host `json:"host"`
}

// Host represents a network device connected or registered on Keenetic router.
type Host struct {
	MAC          string           `json:"mac"`
	IP           string           `json:"ip"`
	Hostname     string           `json:"hostname"`
	Name         string           `json:"name"`
	Registered   bool             `json:"registered"`
	Active       bool             `json:"active"`
	Access       string           `json:"access"` // "permit" or "deny"
	Policy       *string          `json:"policy"`
	Link         string           `json:"link"` // "up" or "down"
	RxBytes      int64            `json:"rxbytes"`
	TxBytes      int64            `json:"txbytes"`
	Uptime       int64            `json:"uptime"`
	TrafficShape HostTrafficShape `json:"traffic-shape"`
	Interface    struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"interface"`
}

// HostTrafficShape holds rate limit settings for a host.
type HostTrafficShape struct {
	Rx       int    `json:"rx"` // kbits/sec
	Tx       int    `json:"tx"` // kbits/sec
	Mode     string `json:"mode"`
	Schedule string `json:"schedule"`
}

// DisplayName returns custom name first, then hostname, then MAC address as fallback.
func (h *Host) DisplayName() string {
	if trimmed := strings.TrimSpace(h.Name); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(h.Hostname); trimmed != "" {
		return trimmed
	}
	return h.MAC
}

// IsBlocked returns true if internet access is blocked ("deny" policy).
func (h *Host) IsBlocked() bool {
	return strings.EqualFold(h.Access, "deny")
}

// Interface represents network interface from /rci/show/interface.
type Interface struct {
	ID            string          `json:"id"`
	InterfaceName string          `json:"interface-name"`
	Type          string          `json:"type"`
	Description   string          `json:"description"`
	Link          string          `json:"link"`
	Connected     string          `json:"connected"` // "yes" or "no"
	State         string          `json:"state"`     // "up" or "down"
	RoleRaw       json.RawMessage `json:"role"`
	Global        bool            `json:"global"`
	Address       *string         `json:"address,omitempty"`
	Mask          *string         `json:"mask,omitempty"`
	DefaultGW     *bool           `json:"defaultgw,omitempty"`
	Fail          *string         `json:"fail,omitempty"`
	Uptime        *int64          `json:"uptime,omitempty"`
	SecurityLevel string          `json:"security-level"`
}

// Roles parses the polymorphic role field from Keenetic.
func (iface *Interface) Roles() []string {
	if len(iface.RoleRaw) == 0 {
		return nil
	}

	var strRoles []string
	if err := json.Unmarshal(iface.RoleRaw, &strRoles); err == nil {
		return strRoles
	}

	var objRoles []struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(iface.RoleRaw, &objRoles); err == nil {
		var res []string
		for _, o := range objRoles {
			if o.Role != "" {
				res = append(res, o.Role)
			}
		}
		return res
	}

	return nil
}

// IsInternetConnection checks if an interface is an external/WAN/VPN connection.
func (iface *Interface) IsInternetConnection() bool {
	// Exclude local hardware ports, bridges, local wifi APs
	switch iface.Type {
	case "Port", "GigabitEthernet", "Bridge", "AccessPoint", "WifiMaster":
		return false
	}

	for _, r := range iface.Roles() {
		if r == "inet" || r == "misc" {
			return true
		}
	}
	if iface.Global || iface.SecurityLevel == "public" {
		return true
	}
	switch iface.Type {
	case "PPPoE", "L2TP", "Wireguard", "OpenVPN", "SSTP", "Ikev2", "UsbModem", "WifiStation":
		return true
	}
	return false
}

// IsConnected returns true if interface connection is established.
func (iface *Interface) IsConnected() bool {
	return strings.EqualFold(iface.Connected, "yes") && strings.EqualFold(iface.State, "up")
}

// SystemInfo represents /rci/show/system info.
type SystemInfo struct {
	Hostname   string `json:"hostname"`
	Domainname string `json:"domainname"`
	CPULoad    int    `json:"cpuload"`
	Memory     string `json:"memory"`
	MemTotal   int64  `json:"memtotal"`
	MemFree    int64  `json:"memfree"`
	Uptime     string `json:"uptime"`
}

// FormatBytes formats byte count into human-readable string (KB, MB, GB).
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// FormatDuration formats seconds into human-readable format.
func FormatDuration(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%d дн. %d ч. %d мин.", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%d ч. %d мин.", hours, mins)
	}
	return fmt.Sprintf("%d мин.", mins)
}
