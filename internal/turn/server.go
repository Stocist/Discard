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

// Start creates and starts an embedded TURN server on UDP.
func Start(realm string) (*turn.Server, error) {
	port := defaultPort
	if p := os.Getenv("TURN_PORT"); p != "" {
		var err error
		port, err = strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid TURN_PORT: %v", err)
		}
	}

	secret := os.Getenv("TURN_SECRET")
	if secret == "" {
		log.Println("WARNING: TURN_SECRET not set, generating random secret (won't persist across restarts)")
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("failed to generate TURN secret: %v", err)
		}
		secret = base64.StdEncoding.EncodeToString(b)
	}

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	udpListener, err := net.ListenPacket("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %v", addr, err)
	}

	s, err := turn.NewServer(turn.ServerConfig{
		Realm: realm,
		AuthHandler: func(username, realm string, srcAddr net.Addr) ([]byte, bool) {
			// Validate HMAC-SHA1 ephemeral credentials
			mac := hmac.New(sha1.New, []byte(secret))
			mac.Write([]byte(username))
			expectedKey := mac.Sum(nil)
			return expectedKey, true
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{PacketConn: udpListener, RelayAddressGenerator: &turn.RelayAddressGeneratorNone{Address: "0.0.0.0"}},
		},
	})
	if err != nil {
		udpListener.Close()
		return nil, fmt.Errorf("failed to start TURN server: %v", err)
	}

	log.Printf("TURN server listening on %s (realm: %s)", addr, realm)
	return s, nil
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
