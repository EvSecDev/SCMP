package helpers

import (
	"fmt"
	"io"
	"net/http"
)

func HTTPCheckResp(response *http.Response) (body []byte, err error) {
	body, err = io.ReadAll(response.Body)
	if err != nil {
		err = fmt.Errorf("failed to read response body: %w", err)
		return
	}

	if response.StatusCode < 200 && response.StatusCode > 299 {
		var respDetails string
		if len(body) == 0 {
			respDetails = "[empty body]"
		} else if len(body) > 1000 {
			respDetails = "[body too large for display]"
		} else {
			respDetails = string(body)
		}
		err = fmt.Errorf("remote sent non-200 status: %d: %s",
			response.StatusCode, respDetails)
		return
	}
	if len(body) == 0 {
		err = fmt.Errorf("received empty response body")
		return
	}
	return
}
