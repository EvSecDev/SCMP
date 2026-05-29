package web

import (
	"fmt"
	"net"
)

// Validates both address and port
// Returns string socket in format of address:port
func validateListenSocket(address string, port int) (socket string, err error) {
	// Validate port
	if port <= 1 || port > 65535 {
		err = fmt.Errorf("invalid port %d", port)
		return
	}

	// Try ipv6, otherwise fallback to ipv4
	if address == "localhost" {
		useIPv6 := false

		ifaces, err := net.Interfaces()
		if err == nil {
			for _, iface := range ifaces {
				addrs, _ := iface.Addrs()
				for _, a := range addrs {
					var ip net.IP
					switch v := a.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					}
					if ip != nil && ip.IsLoopback() && ip.To16() != nil && ip.To4() == nil {
						useIPv6 = true
						break
					}
				}
				if useIPv6 {
					break
				}
			}
		}

		if useIPv6 {
			address = "::1"
		} else {
			address = "127.0.0.1"
		}
	}

	// Validate address
	parsedAddress := net.ParseIP(address)
	if parsedAddress == nil {
		err = fmt.Errorf("invalid address: %s", address)
		return
	}

	// Format socket
	if parsedAddress.To4() != nil {
		socket = fmt.Sprintf("%s:%d", address, port)
	} else if parsedAddress.To16() != nil {
		socket = fmt.Sprintf("[%s]:%d", address, port)
	} else {
		err = fmt.Errorf("unknown address type")
		return
	}

	return
}
