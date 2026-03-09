package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Stocist/discard/internal/models"
	"github.com/google/uuid"
)

// UserRepo is the interface the auth middleware needs to look up and create users.
type UserRepo interface {
	GetByTailscaleID(ctx context.Context, tailscaleID string) (*models.User, error)
	Create(ctx context.Context, user *models.User) error
	// DevAutoJoinServers adds a user to all existing servers (dev mode only).
	DevAutoJoinServers(ctx context.Context, userID uuid.UUID) error
}

// tailscaleWhoisResponse is the subset of the Tailscale localapi whois response we care about.
type tailscaleWhoisResponse struct {
	UserProfile struct {
		ID            int64  `json:"ID"`
		LoginName     string `json:"LoginName"`
		DisplayName   string `json:"DisplayName"`
		ProfilePicURL string `json:"ProfilePicURL"`
	} `json:"UserProfile"`
}

// devUserIDs maps dev user numbers to fixed UUIDs for multi-user testing.
var devUserIDs = [...]uuid.UUID{
	uuid.MustParse("00000000-0000-0000-0000-000000000001"),
	uuid.MustParse("00000000-0000-0000-0000-000000000002"),
	uuid.MustParse("00000000-0000-0000-0000-000000000003"),
	uuid.MustParse("00000000-0000-0000-0000-000000000004"),
}

// Middleware returns an http.Handler that authenticates every request via
// the Tailscale local API (or a hardcoded dev user when DISCARD_DEV=true).
func Middleware(repo UserRepo) func(http.Handler) http.Handler {
	devMode := strings.EqualFold(os.Getenv("DISCARD_DEV"), "true")
	if devMode {
		log.Println("WARNING: Running in dev mode — authentication is disabled. Do NOT use in production.")
	}

	client := &http.Client{Timeout: 3 * time.Second}

	// Tailscale local API: on Linux it's http://127.0.0.1:41112 with no auth.
	// On macOS (App Store) it's a dynamic port with a token.
	// Override via TAILSCALE_API_URL and TAILSCALE_API_TOKEN env vars.
	tsAPIURL := os.Getenv("TAILSCALE_API_URL")
	if tsAPIURL == "" {
		tsAPIURL = "http://127.0.0.1:41112"
	}
	tsAPIToken := os.Getenv("TAILSCALE_API_TOKEN")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			var user *models.User
			var err error

			if devMode {
				// Set dev_user cookie when query param is present
				if q := r.URL.Query().Get("dev_user"); q != "" {
					http.SetCookie(w, &http.Cookie{
						Name:     "dev_user",
						Value:    q,
						Path:     "/",
						MaxAge:   86400,
						HttpOnly: true,
					})
				}
				user, err = devUser(ctx, repo, r)
			} else {
				user, err = tailscaleAuth(ctx, repo, client, r.RemoteAddr, tsAPIURL, tsAPIToken)
			}

			if err != nil {
				log.Printf("auth: %v", err)
				http.Error(w, `{"error":"authentication failed"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(ContextWithUser(ctx, user)))
		})
	}
}

// devUser returns (or auto-creates) a dev user.
// Supports multiple dev users via ?dev_user=N query param (0-3) or dev_user cookie.
// Default is user 0. Access /login?dev_user=1 from a second device to get a different identity.
func devUser(ctx context.Context, repo UserRepo, r *http.Request) (*models.User, error) {
	idx := 0
	// Check query param first, then cookie
	if q := r.URL.Query().Get("dev_user"); q != "" {
		if n := q[0] - '0'; n >= 0 && n < byte(len(devUserIDs)) {
			idx = int(n)
			// Set cookie so subsequent requests (WS, API) use the same user
		}
	} else if c, err := r.Cookie("dev_user"); err == nil {
		if n := c.Value[0] - '0'; n >= 0 && n < byte(len(devUserIDs)) {
			idx = int(n)
		}
	}

	names := [...]string{"Dev", "Alice", "Bob", "Charlie"}
	// idx 0 keeps the original "dev-local" for backwards compat with existing DB rows
	tsID := "dev-local"
	if idx > 0 {
		tsID = fmt.Sprintf("dev-local-%d", idx)
	}

	u, err := repo.GetByTailscaleID(ctx, tsID)
	if err == nil {
		// Idempotent — ensure dev user is a member of all servers (ON CONFLICT DO NOTHING)
		if err := repo.DevAutoJoinServers(ctx, u.ID); err != nil {
			log.Printf("dev: auto-join servers: %v", err)
		}
		return u, nil
	}

	displayName := names[idx]
	now := time.Now()
	u = &models.User{
		ID:          devUserIDs[idx],
		Username:    strings.ToLower(displayName),
		DisplayName: &displayName,
		TailscaleID: &tsID,
		Status:      "online",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := repo.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("create dev user: %w", err)
	}
	// Auto-join all existing servers so the new dev user can test immediately
	if err := repo.DevAutoJoinServers(ctx, u.ID); err != nil {
		log.Printf("dev: auto-join servers: %v", err)
	}
	return u, nil
}

// TailscaleStatus checks if the request comes from a Tailscale user and returns
// their user model (or auto-creates them). Used by the public status endpoint.
func TailscaleStatus(r *http.Request, repo UserRepo) (*models.User, error) {
	client := &http.Client{Timeout: 3 * time.Second}

	tsAPIURL := os.Getenv("TAILSCALE_API_URL")
	if tsAPIURL == "" {
		tsAPIURL = "http://127.0.0.1:41112"
	}
	tsAPIToken := os.Getenv("TAILSCALE_API_TOKEN")

	return tailscaleAuth(r.Context(), repo, client, r.RemoteAddr, tsAPIURL, tsAPIToken)
}

// tailscaleAuth authenticates via the Tailscale local API.
func tailscaleAuth(ctx context.Context, repo UserRepo, client *http.Client, remoteAddr, apiURL, apiToken string) (*models.User, error) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}

	url := fmt.Sprintf("%s/localapi/v0/whois?addr=%s", apiURL, host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build whois request: %w", err)
	}
	if apiToken != "" {
		req.SetBasicAuth("", apiToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tailscale whois: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tailscale whois returned %d", resp.StatusCode)
	}

	var whois tailscaleWhoisResponse
	if err := json.NewDecoder(resp.Body).Decode(&whois); err != nil {
		return nil, fmt.Errorf("decode whois: %w", err)
	}

	if whois.UserProfile.ID == 0 {
		return nil, fmt.Errorf("empty UserProfile.ID from tailscale whois")
	}
	tsID := fmt.Sprintf("%d", whois.UserProfile.ID)

	// Look up existing user.
	u, err := repo.GetByTailscaleID(ctx, tsID)
	if err == nil {
		return u, nil
	}

	// Auto-create on first visit.
	// Use DisplayName as username; never store LoginName (email/PII).
	newID := uuid.New()
	username := whois.UserProfile.DisplayName
	if username == "" {
		username = "User-" + newID.String()[:8]
	}
	displayName := username
	now := time.Now()
	u = &models.User{
		ID:          newID,
		Username:    username,
		DisplayName: &displayName,
		TailscaleID: &tsID,
		Status:      "online",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if whois.UserProfile.ProfilePicURL != "" {
		u.AvatarPath = &whois.UserProfile.ProfilePicURL
	}
	if err := repo.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}
