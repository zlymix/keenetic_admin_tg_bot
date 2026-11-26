package keenetic

import (
	"context"
	"testing"
	"time"
)

func TestClient_LiveRouter(t *testing.T) {
	client := NewClient("http://192.168.1.1", "admin", "tmpPASS123")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 1. Test Login
	t.Run("Login", func(t *testing.T) {
		err := client.Login(ctx)
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}
	})

	// 2. Test GetHotspot
	var sampleMAC string
	t.Run("GetHotspot", func(t *testing.T) {
		hotspot, err := client.GetHotspot(ctx)
		if err != nil {
			t.Fatalf("GetHotspot failed: %v", err)
		}
		if len(hotspot.Host) == 0 {
			t.Fatal("Expected at least one host in hotspot")
		}
		t.Logf("Found %d hosts", len(hotspot.Host))
		for _, h := range hotspot.Host {
			if h.Active && sampleMAC == "" {
				sampleMAC = h.MAC
			}
			t.Logf("Host: %s (IP: %s, MAC: %s, Active: %v, Access: %s)",
				h.DisplayName(), h.IP, h.MAC, h.Active, h.Access)
		}
		if sampleMAC == "" && len(hotspot.Host) > 0 {
			sampleMAC = hotspot.Host[0].MAC
		}
	})

	// 3. Test GetInterfaces
	t.Run("GetInterfaces", func(t *testing.T) {
		ifaces, err := client.GetInterfaces(ctx)
		if err != nil {
			t.Fatalf("GetInterfaces failed: %v", err)
		}
		t.Logf("Found %d total interfaces", len(ifaces))
		for id, iface := range ifaces {
			if iface.IsInternetConnection() {
				addr := "-"
				if iface.Address != nil {
					addr = *iface.Address
				}
				t.Logf("Internet Iface: %s (%s, %s), Connected: %s, State: %s, IP: %s",
					id, iface.Description, iface.Type, iface.Connected, iface.State, addr)
			}
		}
	})

	// 4. Test GetSystemInfo
	t.Run("GetSystemInfo", func(t *testing.T) {
		sys, err := client.GetSystemInfo(ctx)
		if err != nil {
			t.Fatalf("GetSystemInfo failed: %v", err)
		}
		t.Logf("System Info: Hostname: %s, CPU: %d%%, Mem: %s, Uptime: %s",
			sys.Hostname, sys.CPULoad, sys.Memory, sys.Uptime)
	})

	// 5. Test WakeOnLAN
	t.Run("WakeOnLAN", func(t *testing.T) {
		if sampleMAC == "" {
			t.Skip("No sample MAC available")
		}
		err := client.WakeOnLAN(ctx, sampleMAC)
		if err != nil {
			t.Fatalf("WakeOnLAN failed: %v", err)
		}
		t.Logf("WakeOnLAN successfully sent to %s", sampleMAC)
	})

	// 6. Test Speed Limit
	t.Run("SpeedLimit", func(t *testing.T) {
		testMAC := "18:31:bf:52:31:38" // MAX | PC
		// Set 10 Mbps (10240 Kbps)
		err := client.SetDeviceSpeedLimit(ctx, testMAC, 10240)
		if err != nil {
			t.Fatalf("SetDeviceSpeedLimit failed: %v", err)
		}
		t.Logf("Speed limit set to 10240 kbps")

		// Remove speed limit
		err = client.SetDeviceSpeedLimit(ctx, testMAC, 0)
		if err != nil {
			t.Fatalf("Clear speed limit failed: %v", err)
		}
		t.Logf("Speed limit cleared")
	})

	// 7. Test Device Access (Deny & Permit)
	t.Run("DeviceAccess", func(t *testing.T) {
		testMAC := "18:31:bf:52:31:38" // MAX | PC
		// Set Deny (No Internet Access)
		err := client.SetDeviceAccess(ctx, testMAC, false)
		if err != nil {
			t.Fatalf("SetDeviceAccess deny failed: %v", err)
		}
		t.Logf("Access set to deny")

		// Revert to Permit
		err = client.SetDeviceAccess(ctx, testMAC, true)
		if err != nil {
			t.Fatalf("SetDeviceAccess permit failed: %v", err)
		}
		t.Logf("Access set to permit")
	})
}
