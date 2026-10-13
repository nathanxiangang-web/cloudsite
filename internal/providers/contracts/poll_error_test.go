package contracts

import "testing"

func TestUnknownAndDecodeCannotBecomeRetryableByBoolean(t *testing.T) {
	for _, category := range []PollCategory{PollUnknown, PollDecode, PollTransport} {
		if (&PollError{Category: category, Retryable: true, HTTPStatus: 503}).CanRetry() {
			t.Fatal("unsafe retry category")
		}
	}
	for _, status := range []int{0, 200, 401, 404} {
		if (&PollError{Category: PollHTTP, Retryable: true, HTTPStatus: status}).CanRetry() {
			t.Fatal("unsupported retry status")
		}
	}
}
