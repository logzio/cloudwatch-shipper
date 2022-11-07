package logs_processor

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type LogzioSender struct {
	Url        string
	HttpClient *http.Client
}

func initializeSender() (LogzioSender, error) {
	var logzioSender LogzioSender

	token, err := getToken()
	if err != nil {
		return logzioSender, err
	}

	listener, err := getListener()
	if err != nil {
		return logzioSender, err
	}

	client := &http.Client{
		Timeout: getTimeout(),
	}

	sender := LogzioSender{
		Url:        fmt.Sprintf("%s?token=%s&type=%s", listener, token, getType()),
		HttpClient: client,
	}

	//sugLog.Debugf("Using sender: %v", sender)

	return sender, nil
}

func (l *LogzioSender) SendToLogzio(bytesToSend []byte) error {
	var statusCode int
	var compressedBuf bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressedBuf)
	_, err := gzipWriter.Write(bytesToSend)
	if err != nil {
		return err
	}

	err = gzipWriter.Close()
	if err != nil {
		return err
	}

	// retry logic
	backOff := time.Second * 2
	sendRetries := 4
	toBackOff := false
	for attempt := 0; attempt < sendRetries; attempt++ {
		if toBackOff {
			//sugLog.Warnf("Failed to send logs, trying again in %v", backOff)
			time.Sleep(backOff)
			backOff *= 2
		}
		//sugLog.Debugf("Trying to send log: %s", string(bytesToSend))
		statusCode = l.makeHttpRequest(compressedBuf)
		if l.shouldRetry(statusCode) {
			toBackOff = true
		} else {
			break
		}
	}

	if statusCode != 200 {
		//sugLog.Errorf("Error sending logs, status code is: %d", statusCode)
	}

	compressedBuf.Reset()
	return nil
}

func (l *LogzioSender) shouldRetry(statusCode int) bool {
	retry := true
	switch statusCode {
	case http.StatusBadRequest:
		retry = false
	case http.StatusNotFound:
		retry = false
	case http.StatusUnauthorized:
		//sugLog.Error("Please check your Logz.io logs shipping token!")
		retry = false
	case http.StatusForbidden:
		retry = false
	case http.StatusOK:
		retry = false
	}

	//sugLog.Debugf("Got HTTP status code %d. Should retry? %t", statusCode, retry)

	return retry
}

func (l *LogzioSender) makeHttpRequest(data bytes.Buffer) int {
	req, err := http.NewRequest(http.MethodPost, l.Url, &data)
	req.Header.Add("Content-Encoding", "gzip")
	//sugLog.Debugf("Sending bulk of %d bytes", data.Len())
	resp, err := l.HttpClient.Do(req)
	if err != nil {
		//sugLog.Errorf("Error sending logs to %s %s", l.Url, err)
		return 400
	}

	defer resp.Body.Close()
	statusCode := resp.StatusCode
	_, _ = ioutil.ReadAll(resp.Body)
	//if err != nil {
	//	//sugLog.Errorf("Error reading response body: %s", err.Error())
	//}

	//if statusCode < 200 || statusCode > 299 {
	//	//sugLog.Errorf("Response from listener: %s", string(respBody))
	//}

	return statusCode
}
