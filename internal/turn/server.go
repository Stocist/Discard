package turn

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/pion/turn/v4"
)

const defaultPort = 3478
const credentialTTL = 86400 // 24 hours

// Start creates and starts an embedded TURN server on UDP and returns the
// secret the API must use when issuing credentials.
func Start(realm string) (*turn.Server, string, error) {
	port, err := Port()
	if err != nil {
		return nil, "", err
	}

	secret := os.Getenv("TURN_SECRET")
	if secret == "" {
		log.Println("WARNING: TURN_SECRET not set, generating random secret (won't persist across restarts)")
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, "", fmt.Errorf("failed to generate TURN secret: %v", err)
		}
		secret = base64.StdEncoding.EncodeToString(b)
	}

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	udpListener, err := net.ListenPacket("udp4", addr)
	if err != nil {
		return nil, "", fmt.Errorf("failed to listen on %s: %v", addr, err)
	}

	// The relay address is what the TURN server hands back to the browser as
	// the relayed transport address. It MUST be an IP the client can actually
	// reach — bind on 0.0.0.0 but advertise the resolved tailnet/public IP.
	relayIP := resolveRelayIP()

	var relayGen turn.RelayAddressGenerator
	if relayIP != nil {
		relayGen = &turn.RelayAddressGeneratorStatic{RelayAddress: relayIP, Address: "0.0.0.0"}
		log.Printf("TURN relay address: %s", relayIP)
	} else {
		// Last resort: keep startup working, but TURN relay will be unusable
		// (browsers report "TURN server appears to be broken"). Direct/host
		// candidates may still connect.
		relayGen = &turn.RelayAddressGeneratorNone{Address: "0.0.0.0"}
		log.Println("WARNING: could not resolve a reachable TURN relay IP — set TURN_PUBLIC_IP; relay fallback will not work")
	}

	s, err := turn.NewServer(turn.ServerConfig{
		Realm: realm,
		AuthHandler: func(username, realm string, srcAddr net.Addr) ([]byte, bool) {
			// Ephemeral (coturn REST-style) credentials: the client's password
			// is base64(HMAC-SHA1(secret, username)). The browser authenticates
			// with the long-term-credential mechanism, so pion/turn needs the
			// integrity key MD5(username:realm:password) — NOT the raw HMAC.
			mac := hmac.New(sha1.New, []byte(secret))
			mac.Write([]byte(username))
			password := base64.StdEncoding.EncodeToString(mac.Sum(nil))
			return turn.GenerateAuthKey(username, realm, password), true
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{PacketConn: udpListener, RelayAddressGenerator: relayGen},
		},
	})
	if err != nil {
		udpListener.Close()
		return nil, "", fmt.Errorf("failed to start TURN server: %v", err)
	}

	log.Printf("TURN server listening on %s (realm: %s)", addr, realm)
	return s, secret, nil
}

// Port returns the configured UDP listener port.
func Port() (int, error) {
	if value := os.Getenv("TURN_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return 0, fmt.Errorf("invalid TURN_PORT %q: must be between 1 and 65535", value)
		}
		return port, nil
	}
	return defaultPort, nil
}

// resolveRelayIP determines the IP the TURN server should advertise as its
// relay address. Preference order:
//  1. TURN_PUBLIC_IP env override (operator escape hatch)
//  2. A Tailscale CGNAT address (100.64.0.0/10) on any interface — the
//     reachable address in the Docker + Tailscale sidecar deployment
//  3. The first global-unicast non-loopback IPv4 (bare-metal / native install)
//
// Returns nil if nothing suitable is found.
func resolveRelayIP() net.IP {
	if env := os.Getenv("TURN_PUBLIC_IP"); env != "" {
		if ip := net.ParseIP(env); ip != nil {
			return ip
		}
		log.Printf("WARNING: TURN_PUBLIC_IP=%q is not a valid IP, ignoring", env)
	}

	_, tailscaleCGNAT, _ := net.ParseCIDR("100.64.0.0/10")

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var fallback net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil || !ip4.IsGlobalUnicast() {
				continue
			}
			if tailscaleCGNAT.Contains(ip4) {
				return ip4 // best match — reachable over the tailnet
			}
			if fallback == nil {
				fallback = ip4
			}
		}
	}
	return fallback
}

// GenerateCredentials creates time-limited TURN credentials using HMAC-SHA1.
func GenerateCredentials(secret string) (username, credential string, ttl int) {
	ttl = credentialTTL
	timestamp := time.Now().Add(time.Duration(ttl) * time.Second).Unix()

	randBytes := make([]byte, 8)
	rand.Read(randBytes)
	username = fmt.Sprintf("%d:%s", timestamp, base64.RawURLEncoding.EncodeToString(randBytes))

	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	credential = base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return username, credential, ttl
}
