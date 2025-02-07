package util

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var mux = newMux()

func Main(w http.ResponseWriter, r *http.Request) {
	mux.ServeHTTP(w, r)
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/log", logma)
	mux.HandleFunc("/fs", fsHandler)
	mux.HandleFunc("/headers", headersHandler)
	mux.HandleFunc("/info", infoHandler)
	return mux
}

func fsHandler(w http.ResponseWriter, r *http.Request) {
	wd, err := os.Getwd()
	if err != nil {
		http.Error(w, "Error getting working directory", http.StatusInternalServerError)
		return
	}

	var files []string

	addFile := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	}

	err = filepath.Walk(wd, addFile)
	if err != nil {
		http.Error(w, "Error walking directory", http.StatusInternalServerError)
		return
	}

	for _, file := range files {
		fmt.Fprintln(w, file)
	}
}

func logma(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	var message map[string]interface{}
	if err := json.Unmarshal(body, &message); err != nil {
		fmt.Printf("received a logma: %s\n", body)
	} else {
		fmt.Printf("received a logma: %+v\n", message)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "logma success")
}

func infoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	functionName := os.Getenv("FUNCTION_NAME")
	requestMethod := r.Method
	requestPath := r.URL.Path
	queryParams := r.URL.Query()
	environmentVariables := make(map[string]string)

	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		environmentVariables[pair[0]] = pair[1]
	}

	containerID, _ := exec.Command("cat", "/proc/self/cgroup").Output()
	containerIDStr := strings.TrimSpace(strings.Split(string(containerID), "/")[2])
	hostName, _ := exec.Command("hostname").Output()
	ipAddress, _ := exec.Command("hostname", "-I").Output()

	osInfo := runtime.GOOS
	osArch := runtime.GOARCH

	response := map[string]interface{}{
		"functionName":         functionName,
		"requestMethod":        requestMethod,
		"requestPath":          requestPath,
		"queryParams":          queryParams,
		"environmentVariables": environmentVariables,
		"containerID":          containerIDStr,
		"hostName":             strings.TrimSpace(string(hostName)),
		"ipAddress":            strings.TrimSpace(string(ipAddress)),
		"os":                   osInfo,
		"architecture":         osArch,
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s", jsonResponse)
}

func headersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	headers := make(map[string][]string)
	for key, value := range r.Header {
		headers[key] = value
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(headers); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
