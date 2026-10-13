package p115

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/nathanxiangang-web/panta/internal/providers/contracts"
)

// The pinned SDK uses POST for listing. This adapter-local override uses the
// same supported read endpoint with GET, one request and no resty retries. No
// SDK, crypto, creation or cancellation code is changed.
type readOnlyOfflineClient struct{ *driver.Pan115Client }

func (client readOnlyOfflineClient) ListOfflineTask(page int64) (driver.OfflineTaskResp, error) {
	var result driver.OfflineTaskResp
	response, err := client.NewRequest().SetQueryParam("page", strconv.FormatInt(page, 10)).
		SetResult(&result).ForceContentType("application/json;charset=UTF-8").Get(driver.ApiListOfflineUrl)
	if response != nil && response.StatusCode() != 0 && (response.StatusCode() < 200 || response.StatusCode() >= 300) {
		status := response.StatusCode()
		return result, &contracts.PollError{Category: contracts.PollHTTP, HTTPStatus: status,
			Retryable: status == 429 || status >= 500 && status <= 599, Cause: err}
	}
	if err = driver.CheckErr(err, &result, response); err != nil {
		diagnostic := pollDiagnostic(err, 0)
		if response != nil {
			diagnostic.HTTPStatus = response.StatusCode()
		}
		return result, diagnostic
	}
	return result, nil
}

func pollDiagnostic(cause error, elapsed time.Duration) *contracts.PollError {
	var existing *contracts.PollError
	if errors.As(cause, &existing) {
		copy := *existing
		copy.Elapsed = elapsed
		return &copy
	}
	diagnostic := &contracts.PollError{Category: contracts.PollUnknown, Elapsed: elapsed, Cause: cause}
	var network net.Error
	var syntax *json.SyntaxError
	var typed *json.UnmarshalTypeError
	switch {
	case errors.Is(cause, context.Canceled): // cancellation is never an automatic retry
	case errors.As(cause, &network):
		diagnostic.Category = contracts.PollTransport
		if network.Timeout() {
			diagnostic.Category, diagnostic.Retryable = contracts.PollTimeout, true
		}
	case errors.As(cause, &syntax), errors.As(cause, &typed):
		diagnostic.Category = contracts.PollDecode
	}
	return diagnostic
}
