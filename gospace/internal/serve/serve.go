package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
)

var mux = newMux()

func Main(w http.ResponseWriter, r *http.Request) {
	mux.ServeHTTP(w, r)
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/run", runRouterHandler)
	return mux
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Router service is running at /")
}

// RequestPayload defines the expected JSON payload
type RequestPayload struct {
	Port int    `json:"port"`
	Key  string `json:"key"`
}

func runRouterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var req RequestPayload
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if req.Port <= 0 || req.Port > 65535 || req.Key == "" {
		http.Error(w, "Invalid port or key", http.StatusBadRequest)
		return
	}

	// Start a new process to run another instance of the main server
	cmd := exec.Command(os.Args[0], "-port", strconv.Itoa(req.Port), "-key", req.Key)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start router: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]string{
		"message": fmt.Sprintf("Router started on port %d with key '%s'", req.Port, req.Key),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
