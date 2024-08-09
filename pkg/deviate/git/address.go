package git

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/openshift-knative/hack/pkg/deviate/errors"
)

var (
	// ErrInvalidAddress when an invalid address is provided to ParseAddress func.
	ErrInvalidAddress = errors.New("invalid address")
	// See: https://regex101.com/r/IXFQX6/6
	gitAddressRe = regexp.MustCompile(`^(?:([^:]+)://)?(?:([^@]+)@)?([^:]+):(.+?)(?:\.([a-z0-9]+))?$`)
)

// ParseAddress of a GIT remote URL.
func ParseAddress(address string) (*Address, error) {
	u, err := url.Parse(address)
	if err == nil {
		addr := &Address{
			Type:     AddressTypeHTTP,
			Protocol: u.Scheme,
			User:     u.User.String(),
			Host:     u.Host,
			Path:     strings.TrimPrefix(u.Path, "/"),
		}
		const dot = "."
		dotSplit := strings.Split(addr.Path, dot)
		if len(dotSplit) > 1 {
			addr.Ext = dotSplit[len(dotSplit)-1]
			addr.Path = strings.Join(dotSplit[:len(dotSplit)-1], dot)
		}
		return addr, nil
	}

	if matches := gitAddressRe.FindStringSubmatch(address); matches != nil {
		return &Address{
			Type:     AddressTypeGit,
			Protocol: matches[1],
			User:     matches[2],
			Host:     matches[3],
			Path:     matches[4],
			Ext:      matches[5],
		}, nil
	}

	return nil, errors.Wrap(err, ErrInvalidAddress)
}

// AddressType is a type of GIT remote address.
type AddressType int

const (
	// AddressTypeGit is a type of GIT address.
	AddressTypeGit AddressType = iota
	// AddressTypeHTTP is a type of HTTP address.
	AddressTypeHTTP
)

// Address represents a GIT remote address.
type Address struct {
	Type     AddressType
	Protocol string
	User     string
	Host     string
	Path     string
	Ext      string
}
