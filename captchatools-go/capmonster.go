package captchatoolsgo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

/*
   This file will contain the code to interact with capmonster.cloud API
*/

func (c Capmonster) GetToken(additional ...*AdditionalData) (*CaptchaAnswer, error) {
	return c.getCaptchaAnswer(additional...)
}
func (c Capmonster) GetBalance() (float32, error) {
	return c.getBalance()
}

// Method to get Queue ID from the API.
func (c Capmonster) getID(data *AdditionalData) (int, error) {
	// Get Payload
	payload, err := c.createPayload(data)
	if err != nil {
		return 0, err
	}

	// Make request to get answer
	for i := 0; i < 100; i++ {
		resp, err := http.Post("https://api.capmonster.cloud/createTask", "application/json", bytes.NewBuffer([]byte(payload)))
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		response := &capmonsterIDResponse{}
		resp.Body.Close()
		json.Unmarshal(body, response)

		// Parse the response
		if response.ErrorID != 0 { // Means there was an error
			return 0, errCodeToError(response.ErrorCode)
		}
		return response.TaskID, nil
	}
	return 0, ErrMaxAttempts
}

// This method gets the captcha token from the Capmonster API
func (c Capmonster) getCaptchaAnswer(additional ...*AdditionalData) (*CaptchaAnswer, error) {
	var data *AdditionalData = nil
	if len(additional) > 0 {
		data = additional[0]
	}

	// Get Queue ID
	queueID, err := c.getID(data)
	if err != nil {
		return nil, err
	}

	// Get Captcha Answer
	payload, _ := json.Marshal(capmonsterCapAnswerPayload{
		ClientKey: c.config.Api_key,
		TaskID:    queueID,
	})
	response := &capmonsterTokenResponse{}
	for i := 0; i < 100; i++ {
		resp, err := http.Post("https://api.capmonster.cloud/getTaskResult", "application/json", bytes.NewBuffer([]byte(payload)))
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}

		// Parse Response
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		json.Unmarshal(body, response)

		// Check for any errors
		if response.ErrorID != 0 { // means there was an error
			return nil, errCodeToError(response.ErrorCode)
		}

		// Check if captcha is ready
		if response.Status == "processing" {
			time.Sleep(3 * time.Second)
			continue
		}

		solution := response.Solution.GRecaptchaResponse
		if c.config.CaptchaType == ImageCaptcha {
			solution = response.Solution.Text
		}
		return newCaptchaAnswer(
			queueID,
			solution,
			c.config.Api_key,
			c.config.CaptchaType,
			AnticaptchaSite,
		), nil
	}
	return nil, ErrMaxAttempts
}

// getBalance() returns the balance on the API key
func (c Capmonster) getBalance() (float32, error) {
	// Attempt to get the balance from the API
	// Max attempts is 5
	payload := fmt.Sprintf(`{"clientKey": "%v"}`, c.config.Api_key)
	response := &capmonsterBalanceResponse{}
	for i := 0; i < 5; i++ {
		resp, err := http.Post("https://api.capmonster.cloud/getBalance", "application/json", bytes.NewBuffer([]byte(payload)))
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		// Parse Response
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		json.Unmarshal(body, response)
		if response.ErrorID != 0 {
			return 0, errCodeToError(response.ErrorCode)
		}
		return response.Balance, nil
	}
	return 0, ErrMaxAttempts
}

/*
createPayload returns the payloads required to interact with the API.

Possible errors that can be returned:
1) ErrIncorrectCapType
*/
func (c Capmonster) createPayload(data *AdditionalData) (string, error) {
	// Define the payload we are going to send to the API
	payload := capmonsterIDPayload{
		ClientKey: c.config.Api_key,
		Task: struct {
			WebsiteURL    string      "json:\"websiteURL\""
			WebsiteKey    string      "json:\"websiteKey\""
			Type          captchaType "json:\"type\""
			IsInvisible   bool        "json:\"isInvisible,omitempty\""
			MinScore      float32     "json:\"minScore,omitempty\""
			PageAction    string      "json:\"pageAction,omitempty\""
			Body          string      "json:\"body,omitempty\""
			ProxyType     string      "json:\"proxyType,omitempty\""
			ProxyAddress  string      "json:\"proxyAddress,omitempty\""
			ProxyPort     int         "json:\"proxyPort,omitempty\""
			ProxyLogin    string      "json:\"proxyLogin,omitempty\""
			ProxyPassword string      "json:\"proxyPassword,omitempty\""
			UserAgent     string      "json:\"userAgent,omitempty\""
		}{
			WebsiteURL: c.config.CaptchaURL,
			WebsiteKey: c.config.Sitekey,
			Type:       c.config.CaptchaType,
		},
	}

	// Add any other keys to the payload
	switch c.config.CaptchaType {
	case ImageCaptcha:
		if data == nil {
			return "", ErrAddionalDataMissing
		}
		payload.Task.Type = "ImageToTextTask"
		payload.Task.Body = data.B64Img
	case V2Captcha:
		payload.Task.Type = "NoCaptchaTaskProxyless"

		// Check for proxy data
		if data != nil && data.Proxy != nil {
			payload.Task.Type = "NoCaptchaTask"
			if data.ProxyType == "" {
				data.ProxyType = "http"
			}
			payload.Task.ProxyType = data.ProxyType
			payload.Task.ProxyAddress = data.Proxy.Ip
			portInt, err := strconv.Atoi(data.Proxy.Port)
			if err != nil {
				return "", errors.New("error converting proxy port to int")
			}
			payload.Task.ProxyPort = portInt
			if data.Proxy.IsUserAuth() {
				payload.Task.ProxyLogin = data.Proxy.User
				payload.Task.ProxyPassword = data.Proxy.Password
			}
		}

		if c.config.IsInvisibleCaptcha {
			payload.Task.IsInvisible = c.config.IsInvisibleCaptcha
		}
	case V3Captcha:
		payload.Task.Type = "RecaptchaV3TaskProxyless"
		payload.Task.MinScore = c.config.MinScore
		payload.Task.PageAction = c.config.Action
	case HCaptcha:
		payload.Task.Type = "HCaptchaTaskProxyless"
	default:
		return "", ErrIncorrectCapType
	}

	// Check for addtional data
	if data != nil && data.UserAgent != "" {
		payload.Task.UserAgent = data.UserAgent
	}

	encoded, _ := json.Marshal(payload)
	return string(encoded), nil
}
