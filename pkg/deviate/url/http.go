package url

import "strings"

const (
	httpURL  = "http://"
	httpsURL = "https://"
)

// IsHTTP returns true if the provided URL is HTTP.
func IsHTTP(url string) bool {
	return strings.HasPrefix(url, httpsURL) ||
		strings.HasPrefix(url, httpURL)
}
