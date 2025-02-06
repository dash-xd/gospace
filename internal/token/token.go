package gauth

import (
  "context"
	"fmt"
	"io"
	"net/http"
	"os"
  "encoding/json"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
)

var mux = newMux()

func Main(w http.ResponseWriter, r *http.Request) {
	mux.ServeHTTP(w, r)
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/id", idTokenHandler)
  mux.HandleFunc("/access", accessTokenHandler)
	return mux
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "gauth service is running at /")
}
func idTokenHandler(w http.ResponseWriter, r *http.Request) {
	url := os.Getenv("ID_TOKEN_SOURCE_URL")

	ctx := r.Context()

	credentials, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to generate default credentials: %v", err), http.StatusInternalServerError)
		return
	}

	ts, err := idtoken.NewTokenSource(ctx, url, option.WithCredentials(credentials))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create NewTokenSource: %v", err), http.StatusInternalServerError)
		return
	}

	token, err := ts.Token()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to receive token: %v", err), http.StatusInternalServerError)
		return
	}

	newToken := token.WithExtra(map[string]interface{}{
		"custom_key": "custom_value",
	})

	tokenInfoURL := fmt.Sprintf("https://oauth2.googleapis.com/tokeninfo?id_token=%s", newToken.AccessToken)

	resp, err := http.Get(tokenInfoURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch token info: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read token info response: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(resp.StatusCode)
	_, err = w.Write(body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to write response: %v", err), http.StatusInternalServerError)
		return
	}
}

func accessTokenHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	credentials, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to generate default credentials: %v", err), http.StatusInternalServerError)
		return
	}

	token, err := credentials.TokenSource.Token()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to retrieve access token: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"access_token":  token.AccessToken,
		"token_type":    token.TokenType,
		"refresh_token": token.RefreshToken,
		"expiry":        token.Expiry.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to encode response as JSON: %v", err), http.StatusInternalServerError)
		return
	}
}
