package oidc

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/ONSdigital/census31-eq-questionnaire-launcher/settings"
	"github.com/patrickmn/go-cache"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
)

func generateIdToken(clientIdName string) (oauth2.TokenSource, error) {
	oidcBackend := settings.Get("OIDC_TOKEN_BACKEND")
	if oidcBackend == "gcp" {
		audience := settings.Get(clientIdName)
		if audience == "" {
			return nil, fmt.Errorf("%s not set", clientIdName)
		}
		return getGCPIdToken(audience, clientIdName)
	}
	return nil, nil
}

func cachedWithTTL(fn func(audience string, clientIdName string) (oauth2.TokenSource, error)) func(audience string, clientIdName string) (oauth2.TokenSource, error) {
	validitySeconds, _ := strconv.Atoi(settings.Get("OIDC_TOKEN_VALIDITY_IN_SECONDS"))
	leewaySeconds, _ := strconv.Atoi(settings.Get("OIDC_TOKEN_LEEWAY_IN_SECONDS"))

	ttl := validitySeconds - leewaySeconds
	// Create cache with default expiration of TTL seconds and cleanup interval of 1 minute
	ttlCache := cache.New(time.Duration(ttl)*time.Second, time.Minute)

	cachedFunc := func(audience string, clientIdName string) (oauth2.TokenSource, error) {
		cachedSource, found := ttlCache.Get(audience)
		if found {
			log.Printf("Found cached GCP ID token source for %s audience: %s", clientIdName, audience)
			return cachedSource.(oauth2.TokenSource), nil
		}
		tokenSource, err := fn(audience, clientIdName)
		if err != nil {
			return nil, err
		}
		ttlCache.Set(audience, tokenSource, cache.DefaultExpiration)
		return tokenSource, nil
	}
	return cachedFunc
}

// uses the Google Cloud metadata server environment to create an identity token that can be added to a HTTP request
// based off https://cloud.google.com/docs/authentication/get-id-token#go
func getIdTokenFromMetadataServer(audience string, clientIdName string) (oauth2.TokenSource, error) {
	ctx := context.Background()
	// Construct the GoogleCredentials object which obtains the default configuration from your working environment.
	credentials, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate default credentials for %s: %w", clientIdName, err)
	}

	ts, err := idtoken.NewTokenSource(ctx, audience, option.WithCredentials(credentials))
	if err != nil {
		return nil, fmt.Errorf("failed to create NewTokenSource for %s: %w", clientIdName, err)
	}

	// Generate the ID token.
	_, err = ts.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to receive token for %s: %w", clientIdName, err)
	}
	log.Printf("Successfully generated GCP ID token for %s audience: %s", clientIdName, audience)

	return ts, nil
}

func ConfigureClientAuthentication(client *http.Client, clientIdName string) (*http.Client, error) {
	tokenSource, err := generateIdToken(clientIdName)
	if err != nil {
		return client, err
	}

	if tokenSource != nil {
		client.Transport = &oauth2.Transport{
			Source: tokenSource,
		}
	}
	return client, nil
}

var getGCPIdToken = cachedWithTTL(getIdTokenFromMetadataServer)
