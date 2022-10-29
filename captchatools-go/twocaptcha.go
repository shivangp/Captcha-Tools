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

// This file will contain the code to interact with anticaptcha.com API

func (t Twocaptcha) GetToken() (*CaptchaAnswer, error) {
	return t.getCaptchaAnswer()
}
func (t Twocaptcha) GetBalance() (float32, error) {
	return t.getBalance()
}

// Method to get Queue ID from the API.
func (t Twocaptcha) getID() (string, error) {
	// Get Payload
	payload, _ := t.createPayload()

	// Make request to get answer
	for {
		resp, err := http.Post("http://2captcha.com/in.php", "application/json", bytes.NewBuffer([]byte(payload)))
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		response := &twocaptchaResponse{}
		resp.Body.Close()
		json.Unmarshal(body, response)

		// Parse the response
		if response.Status != 1 { // Means there was an error
			return "", errCodeToError(response.Request)
		}
		return response.Request, nil
	}
}

// This method gets the captcha token from the Capmonster API
func (t Twocaptcha) getCaptchaAnswer() (*CaptchaAnswer, error) {
	// Get Queue ID
	queueID, err := t.getID()
	if err != nil {
		return nil, err
	}

	// Get Captcha Answer
	response := &twocaptchaResponse{}
	urlToAnswer := fmt.Sprintf(
		"http://2captcha.com/res.php?key=%v&action=get&id=%v&json=1",
		t.config.Api_key,
		queueID,
	)
	for {
		resp, err := http.Get(urlToAnswer)
		if err != nil {
			time.Sleep(3 * time.Second)
			continue
		}

		// Parse Response
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		json.Unmarshal(body, response)
		if response.Status == 1 {
			return newCaptchaAnswer(
				queueID,
				response.Request,
				t.config.Api_key,
				TwoCaptchaSite,
			), nil
		} else if response.Request == "ERROR_CAPTCHA_UNSOLVABLE" {
			t.GetToken()
		}
		time.Sleep(3 * time.Second)
	}
}

func (t Twocaptcha) getBalance() (float32, error) {
	// Attempt to get the balance from the API
	// Max attempts is 5
	url := fmt.Sprintf("https://2captcha.com/res.php?key=%v&action=getbalance&json=1", t.config.Api_key)
	response := &twocaptchaResponse{}
	for i := 0; i < 5; i++ {
		resp, err := http.Get(url)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		// Parse Response
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		json.Unmarshal(body, response)
		if response.Status == 0 {
			return 0, errCodeToError(response.Request)
		}

		// Convert to float32
		var balance float32
		value, err := strconv.ParseFloat(response.Request, 32)
		if err != nil {
			return 0, errors.New("unable to convert balance")
		}
		balance = float32(value)
		return balance, nil
	}
	return 0, ErrMaxAttempts
}

/*
createPayload returns the payloads required to interact with the API.

Possible errors that can be returned:
1) ErrIncorrectCapType
*/
func (t Twocaptcha) createPayload() (string, error) {
	// Define the payload we are going to send to the API
	payload := twoCapIDPayload{
		Key:     t.config.Api_key,
		Pageurl: t.config.CaptchaURL,
		JSON:    1,
		Method:  "userrecaptcha",
	}

	// Add any other keys to the payload
	if t.config.SoftID != 0 {
		payload.SoftID = t.config.SoftID
	}
	switch t.config.CaptchaType {
	case "v2":
		payload.Googlekey = t.config.Sitekey
		if t.config.IsInvisibleCaptcha {
			payload.Invisible = 1
		}
	case "v3":
		payload.Googlekey = t.config.Sitekey
		payload.Version = "v3"
		payload.Action = t.config.Action
		payload.MinScore = t.config.MinScore
	case "hcaptcha", "hcap":
		payload.Method = "hcaptcha"
		payload.Sitekey = t.config.Sitekey
	default:
		return "", ErrIncorrectCapType
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded), nil
}
