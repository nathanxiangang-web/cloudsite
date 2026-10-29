package acquisition

import (
	"encoding/base32"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

const MaxSourceInputLength = 64 << 10

type SourcePolicy string

const (
	SourceExactV1    SourcePolicy = "source-exact-v1"
	MagnetBTIHNameV1 SourcePolicy = "magnet-btih-name-v1"
)

var (
	ErrSourcePolicy      = errors.New("invalid acquisition source policy")
	ErrMagnetAmbiguous   = errors.New("ambiguous magnet source")
	ErrMagnetUnsupported = errors.New("unsupported magnet semantics")
)

// PreparedSource is private admission input, not an HTTP/logging DTO. A future
// admission must atomically persist Original, Reference.Value and Policy before
// activation. This helper is deliberately NOT wired into legacy submission.
type PreparedSource struct {
	Original          string                    `json:"-"`
	Reference         contracts.SourceReference `json:"-"`
	Policy            SourcePolicy              `json:"policy"`
	Changed           bool                      `json:"changed"`
	RemovedParameters int                       `json:"removed_parameters"`
}

func (PreparedSource) String() string { return "prepared acquisition source (redacted)" }

// PrepareSource performs no I/O and never guesses what a provider supports.
// The caller selects a frozen server-side policy; workers never re-prepare.
func PrepareSource(value string, policy SourcePolicy) (PreparedSource, error) {
	if policy != SourceExactV1 && policy != MagnetBTIHNameV1 {
		return PreparedSource{}, ErrSourcePolicy
	}
	if value == "" || len(value) > MaxSourceInputLength || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
		return PreparedSource{}, ErrInvalidSource
	}
	colon := strings.IndexByte(value, ':')
	if policy == SourceExactV1 || colon < 0 || !strings.EqualFold(value[:colon], "magnet") {
		ref, err := ResolveSource(value)
		if err != nil {
			return PreparedSource{}, err
		}
		return PreparedSource{Original: value, Reference: ref, Policy: policy}, nil
	}
	original := value
	value, removed, err := trimMagnetAMPTail(value)
	if err != nil {
		return PreparedSource{}, err
	}
	query, err := magnetQuery(value)
	if err != nil {
		return PreparedSource{}, ErrInvalidSource
	}
	hash, name := "", ""
	nameSeen := false
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values := query[key]
		switch strings.ToLower(key) {
		case "xt":
			for _, topic := range values {
				next, err := magnetBTIH(topic)
				if err != nil {
					return PreparedSource{}, err
				}
				if hash != "" {
					if hash != next {
						return PreparedSource{}, ErrMagnetAmbiguous
					}
					removed++
				}
				hash = next
			}
		case "dn":
			for _, next := range values {
				if !utf8.ValidString(next) || strings.ContainsFunc(next, unicode.IsControl) {
					return PreparedSource{}, ErrInvalidSource
				}
				if nameSeen {
					if name != next {
						return PreparedSource{}, ErrMagnetAmbiguous
					}
					removed++
				}
				name = next
				nameSeen = true
			}
		case "so":
			// Select-only changes the set of files, not just tracker discovery. Do not
			// turn a selective download into a whole-torrent task by dropping it.
			return PreparedSource{}, ErrMagnetUnsupported
		default:
			removed += len(values)
		}
	}
	if hash == "" {
		return PreparedSource{}, ErrInvalidSource
	}
	execution := "magnet:?xt=urn:btih:" + hash
	if name != "" {
		execution += "&dn=" + strings.ReplaceAll(url.QueryEscape(name), "+", "%20")
	} else if nameSeen {
		removed++
	}
	ref, err := ResolveSource(execution)
	if err != nil {
		return PreparedSource{}, err
	}
	return PreparedSource{Original: original, Reference: ref, Policy: policy, Changed: execution != original, RemovedParameters: removed}, nil
}

func magnetQuery(value string) (url.Values, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || parsed.Host != "" || parsed.Path != "" || parsed.Fragment != "" || strings.Contains(value, "#") || parsed.RawQuery == "" || strings.Count(parsed.RawQuery, "&") >= 512 {
		return nil, ErrInvalidSource
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return nil, ErrInvalidSource
	}
	return query, nil
}

// trimMagnetAMPTail handles copied HTML separators without decoding parameter
// data. The retained prefix must still contain a valid BTIH. Inspect content
// selectors across the entire input before discarding any advisory tail.
func trimMagnetAMPTail(value string) (string, int, error) {
	cut := strings.Index(value, "&amp;")
	if cut < 0 {
		return value, 0, nil
	}
	query, err := magnetQuery(strings.ReplaceAll(value, "&amp;", "&"))
	if err != nil {
		return "", 0, err
	}
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := ""
	for _, key := range keys {
		switch strings.ToLower(key) {
		case "so":
			return "", 0, ErrMagnetUnsupported
		case "xt":
			for _, topic := range query[key] {
				next, err := magnetBTIH(topic)
				if err != nil {
					return "", 0, err
				}
				if hash != "" && hash != next {
					return "", 0, ErrMagnetAmbiguous
				}
				hash = next
			}
		}
	}
	removed := 0
	for _, parameter := range strings.Split(strings.ReplaceAll(value[cut:], "&amp;", "&"), "&") {
		if parameter != "" {
			removed++
		}
	}
	return value[:cut], removed, nil
}

func magnetBTIH(topic string) (string, error) {
	const prefix = "urn:btih:"
	if len(topic) < len(prefix) || !strings.EqualFold(topic[:len(prefix)], prefix) {
		return "", ErrMagnetUnsupported
	}
	hash := topic[len(prefix):]
	var data []byte
	var err error
	switch len(hash) {
	case 40:
		data, err = hex.DecodeString(hash)
	case 32:
		data, err = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(hash))
	default:
		return "", ErrInvalidSource
	}
	if err != nil || len(data) != 20 {
		return "", ErrInvalidSource
	}
	return hex.EncodeToString(data), nil
}
