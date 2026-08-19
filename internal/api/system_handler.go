package api

import (
	"net"
	"net/http"
)

// handleSystemAddresses returns the machine's non-loopback IPv4 addresses so
// the frontend can offer an external (LAN) host when building connection
// strings for a local instance.
func (s *Server) handleSystemAddresses(w http.ResponseWriter, r *http.Request) {
	addrs := localIPv4Addresses()
	if addrs == nil {
		addrs = []string{}
	}
	writeOK(w, addrs)
}

func localIPv4Addresses() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ip4 := ipnet.IP.To4(); ip4 != nil && !ip4.IsLoopback() {
				out = append(out, ip4.String())
			}
		}
	}
	return out
}
