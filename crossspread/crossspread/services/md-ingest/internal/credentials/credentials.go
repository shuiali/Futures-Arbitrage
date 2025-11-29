package credentials

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// ExchangeCredentials holds decrypted API credentials for an exchange
type ExchangeCredentials struct {
	APIKey     string `json:"apiKey"`
	APISecret  string `json:"apiSecret"`
	Passphrase string `json:"passphrase,omitempty"`
	UserID     string `json:"userId"`
}

// CredentialsFetcher fetches API credentials from the backend API
type CredentialsFetcher struct {
	backendURL    string
	serviceSecret string
	httpClient    *http.Client
}

// NewCredentialsFetcher creates a new credentials fetcher
func NewCredentialsFetcher(backendURL, serviceSecret string) *CredentialsFetcher {
	return &CredentialsFetcher{
		backendURL:    backendURL,
		serviceSecret: serviceSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetAllCredentials fetches all credentials grouped by exchange
func (f *CredentialsFetcher) GetAllCredentials() (map[string][]ExchangeCredentials, error) {
	url := fmt.Sprintf("%s/api/v1/internal/credentials", f.backendURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Service %s", f.serviceSecret))
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch credentials: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized: invalid service credentials")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string][]ExchangeCredentials
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// GetExchangeCredentials fetches credentials for a specific exchange
func (f *CredentialsFetcher) GetExchangeCredentials(exchange string) ([]ExchangeCredentials, error) {
	url := fmt.Sprintf("%s/api/v1/internal/credentials/%s", f.backendURL, exchange)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Service %s", f.serviceSecret))
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch credentials: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized: invalid service credentials")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result []ExchangeCredentials
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// HasCredentials checks if any credentials exist for the given exchange
func (f *CredentialsFetcher) HasCredentials(exchange string) bool {
	creds, err := f.GetExchangeCredentials(exchange)
	if err != nil {
		log.Warn().Err(err).Str("exchange", exchange).Msg("Failed to check credentials")
		return false
	}
	return len(creds) > 0
}

// GetFirstCredentials returns the first set of credentials for an exchange (for single-user setups)
func (f *CredentialsFetcher) GetFirstCredentials(exchange string) (*ExchangeCredentials, error) {
	creds, err := f.GetExchangeCredentials(exchange)
	if err != nil {
		return nil, err
	}

	if len(creds) == 0 {
		return nil, fmt.Errorf("no credentials found for exchange %s", exchange)
	}

	return &creds[0], nil
}
