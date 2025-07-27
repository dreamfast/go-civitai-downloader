package api

import (
	"bufio"
	"bytes"
	"fmt"
	"go-civitai-download/internal/helpers"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	sync "sync"
	time "time"

	log "github.com/sirupsen/logrus"
)

// Global slice to keep track of all logging transports created
var (
	activeLoggingTransports []*LoggingTransport
	transportsMu            sync.Mutex
)

// LoggingTransport wraps an http.RoundTripper to log request and response details.
type LoggingTransport struct {
	Transport http.RoundTripper
	logFile   *os.File
	mu        sync.Mutex
	writer    *bufio.Writer
}

// NewLoggingTransport creates a new LoggingTransport.
// It opens the specified log file for appending.
func NewLoggingTransport(transport http.RoundTripper, logFilePath string) (*LoggingTransport, error) {
	safeLogFilePath := helpers.SanitizePath(logFilePath)
	// #nosec G304
	f, err := os.OpenFile(safeLogFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open API log file %s: %w", safeLogFilePath, err)
	}

	// Use default transport if none provided
	if transport == nil {
		transport = http.DefaultTransport
	}

	lt := &LoggingTransport{
		Transport: transport,
		logFile:   f,
		writer:    bufio.NewWriter(f), // Use a buffered writer
	}

	// Register the new transport
	transportsMu.Lock()
	activeLoggingTransports = append(activeLoggingTransports, lt)
	transportsMu.Unlock()
	log.Debugf("Registered new LoggingTransport for file: %s. Total active: %d", logFilePath, len(activeLoggingTransports))

	return lt, nil
}

// RoundTrip executes a single HTTP transaction, logging details.
func (t *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	log.Debug("[LogTransport] RoundTrip: Entered") // VERBOSE
	startTime := time.Now()

	// Log request
	log.Debug("[LogTransport] RoundTrip: Dumping request...") // VERBOSE
	reqDump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		log.WithError(err).Error("[LogTransport] Failed to dump API request for logging")
		// Proceed with the request anyway
	} else {
		log.Debug("[LogTransport] RoundTrip: Writing request dump...") // VERBOSE
		t.writeLog(fmt.Sprintf("--- Request (%s) ---\n%s\n", startTime.Format(time.RFC3339), string(reqDump)))
		log.Debug("[LogTransport] RoundTrip: Request dump written.") // VERBOSE
	}

	// Perform the actual request
	log.Debug("[LogTransport] RoundTrip: Performing underlying Transport.RoundTrip...") // VERBOSE
	resp, err := t.Transport.RoundTrip(req)
	log.Debugf("[LogTransport] RoundTrip: Underlying Transport.RoundTrip returned. Err: %v", err) // VERBOSE

	duration := time.Since(startTime)

	// Log response or error
	if err != nil {
		log.Debug("[LogTransport] RoundTrip: Writing response error...") // VERBOSE
		t.writeLog(fmt.Sprintf("--- Response Error (%s, Duration: %v) ---\n%s\n", time.Now().Format(time.RFC3339), duration, err.Error()))
		log.Debug("[LogTransport] RoundTrip: Response error written.") // VERBOSE
	} else {
		log.Debug("[LogTransport] RoundTrip: Processing response...") // VERBOSE
		// Check Content-Type to decide whether to log body
		contentType := resp.Header.Get("Content-Type")
		logBody := strings.HasPrefix(contentType, "application/json")
		log.Debugf("[LogTransport] RoundTrip: Response Content-Type: %s, LogBody: %t", contentType, logBody) // VERBOSE

		if logBody {
			log.Debug("[LogTransport] RoundTrip: Reading response body for logging...") // VERBOSE
			// Read the body for logging
			bodyBytes, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				log.WithError(readErr).Error("[LogTransport] Failed to read response body for logging")
				// Log headers only if body read fails
				respDump, dumpErr := httputil.DumpResponse(resp, false) // Headers only
				if dumpErr != nil {
					log.WithError(dumpErr).Error("[LogTransport] Failed to dump response headers after body read error")
					t.writeLog(fmt.Sprintf("--- Response Headers (%s, Duration: %v) ---\nStatus: %s\n(Failed to dump headers or read body)\n", time.Now().Format(time.RFC3339), duration, resp.Status))
				} else {
					log.Debug("[LogTransport] RoundTrip: Writing response headers (body read failed)...") // VERBOSE
					t.writeLog(fmt.Sprintf("--- Response Headers (%s, Duration: %v) ---\n%s\n(Body read failed)\n", time.Now().Format(time.RFC3339), duration, string(respDump)))
					log.Debug("[LogTransport] RoundTrip: Response headers written (body read failed).") // VERBOSE
				}
				// Restore body with an empty reader? Or let the original error propagate?
				// Let the caller handle the read error; resp.Body is likely closed or unusable now.
			} else {
				log.Debug("[LogTransport] RoundTrip: Response body read successfully. Restoring body...") // VERBOSE
				// IMPORTANT: Restore the body so the caller can read it.
				if closeErr := resp.Body.Close(); closeErr != nil {
					// Log the error but don't necessarily stop the process, as body might still be readable by caller
					log.WithError(closeErr).Warn("[LogTransport] Failed to close original response body before replacing it")
				}
				// Use bytes.NewReader instead of bytes.NewBuffer for the replacement body
				resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

				// Log headers and the body we read
				respDumpHeader, dumpErr := httputil.DumpResponse(resp, false) // Headers only
				if dumpErr != nil {
					log.WithError(dumpErr).Error("[LogTransport] Failed to dump response headers for logging")
					t.writeLog(fmt.Sprintf("--- Response (%s, Duration: %v) ---\nStatus: %s\n(Failed to dump headers, body logged below)\n%s\n", time.Now().Format(time.RFC3339), duration, resp.Status, string(bodyBytes)))
				} else {
					log.Debug("[LogTransport] RoundTrip: Writing response headers and body...") // VERBOSE
					// Log headers first, then body for clarity
					t.writeLog(fmt.Sprintf("--- Response Headers (%s, Duration: %v) ---\n%s\n--- Response Body (%s) ---\n%s\n", time.Now().Format(time.RFC3339), duration, string(respDumpHeader), contentType, string(bodyBytes)))
					log.Debug("[LogTransport] RoundTrip: Response headers and body written.") // VERBOSE
				}
			}
		} else {
			// Log only headers for non-JSON content types
			log.Debug("[LogTransport] RoundTrip: Dumping non-JSON response headers...") // VERBOSE
			respDump, dumpErr := httputil.DumpResponse(resp, false)
			if dumpErr != nil {
				log.WithError(dumpErr).Error("[LogTransport] Failed to dump non-JSON response headers for logging")
				t.writeLog(fmt.Sprintf("--- Response Headers (%s, Duration: %v, Type: %s) ---\nStatus: %s\n(Failed to dump headers)\n", time.Now().Format(time.RFC3339), duration, contentType, resp.Status))
			} else {
				log.Debug("[LogTransport] RoundTrip: Writing non-JSON response headers...") // VERBOSE
				t.writeLog(fmt.Sprintf("--- Response Headers (%s, Duration: %v, Type: %s) ---\n%s\n(Body not logged)\n", time.Now().Format(time.RFC3339), duration, contentType, string(respDump)))
				log.Debug("[LogTransport] RoundTrip: Non-JSON response headers written.") // VERBOSE
			}
		}
	}

	// Ensure logs are written **immediately** after each request/response pair
	log.Debug("[LogTransport] RoundTrip: Flushing writer...") // VERBOSE
	if errFlush := t.writer.Flush(); errFlush != nil {
		// Log error if flushing fails
		log.WithError(errFlush).Error("[LogTransport] Failed to flush log writer")
	}
	log.Debug("[LogTransport] RoundTrip: Writer flushed.") // VERBOSE

	log.Debug("[LogTransport] RoundTrip: Exiting") // VERBOSE
	return resp, err
}

// writeLog writes a string to the buffered writer.
func (t *LoggingTransport) writeLog(logString string) {
	_, err := t.writer.WriteString(logString + "\n\n")
	if err != nil {
		// Log to stderr if writing to file fails
		fmt.Fprintf(os.Stderr, "Error writing to API log file: %v\nLog message: %s\n", err, logString)
	}
}

// Close closes the underlying log file.
func (t *LoggingTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	errFlush := t.writer.Flush() // Ensure buffer is flushed before closing
	errClose := t.logFile.Close()
	if errFlush != nil {
		return fmt.Errorf("failed to flush API log buffer: %w", errFlush)
	}
	return errClose // Return close error if flush was successful
}

// CloseAllLoggingTransports iterates over all created transports and closes them.
func CloseAllLoggingTransports() {
	transportsMu.Lock()
	defer transportsMu.Unlock()

	log.Debugf("Attempting to close %d active logging transports.", len(activeLoggingTransports))
	closedCount := 0
	for i, t := range activeLoggingTransports {
		log.Debugf("Closing transport #%d for file: %s", i+1, t.logFile.Name())
		if err := t.Close(); err != nil {
			// Log error to stderr as the primary logger might also be closing
			fmt.Fprintf(os.Stderr, "Error closing logging transport for %s: %v\n", t.logFile.Name(), err)
		} else {
			closedCount++
		}
	}
	log.Debugf("Successfully closed %d logging transports.", closedCount)
	// Clear the slice after closing
	activeLoggingTransports = []*LoggingTransport{}
}

// DeregisterLoggingTransport removes a specific transport from the active list.
// This might be useful if a transport needs to be manually closed and removed earlier.
// Note: Ensure Close() is called separately if needed before deregistering.
func DeregisterLoggingTransport(transportToDeregister *LoggingTransport) {
	transportsMu.Lock()
	defer transportsMu.Unlock()

	log.Debugf("Attempting to deregister logging transport for file: %s", transportToDeregister.logFile.Name())
	found := false
	newActiveTransports := []*LoggingTransport{}
	for _, t := range activeLoggingTransports {
		if t != transportToDeregister {
			newActiveTransports = append(newActiveTransports, t)
		} else {
			found = true
		}
	}
	activeLoggingTransports = newActiveTransports
	if found {
		log.Debugf("Successfully deregistered transport. Remaining active: %d", len(activeLoggingTransports))
	} else {
		log.Warnf("Attempted to deregister a transport that was not found.")
	}
}
