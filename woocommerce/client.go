package woocommerce

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	Version       = "1.0.0"
	UserAgent     = "WooCommerce API Client-PHP/" + Version
	HashAlgorithm = "HMAC-SHA256"
)

type Client struct {
	storeURL  *url.URL
	ck        string
	cs        string
	option    *Option
	rawClient *http.Client
}

func NewClient(store, ck, cs string, option *Option) (*Client, error) {
	storeURL, err := url.Parse(store)
	if err != nil {
		return nil, err
	}

	if option == nil {
		option = &Option{}
	}
	if option.OauthTimestamp.IsZero() {
		option.OauthTimestamp = time.Now()
	}

	ver := "v3"
	if option.Version != "" {
		ver = option.Version
	}
	path := "/wc-api/"
	if option.API {
		path = option.APIPrefix
	}
	path = path + ver + "/"
	storeURL.Path = path

	rawClient := &http.Client{}
	if !option.VerifySSL {
		rawClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return &Client{
		storeURL:  storeURL,
		ck:        ck,
		cs:        cs,
		option:    option,
		rawClient: rawClient,
	}, nil
}

func (c *Client) basicAuth(params url.Values) string {
	if params == nil {
		params = url.Values{}
	}
	params.Add("consumer_key", c.ck)
	params.Add("consumer_secret", c.cs)
	return params.Encode()
}

func (c *Client) oauth(method, urlStr string, params url.Values) string {
	if params == nil {
		params = make(url.Values)
	}
	params.Add("oauth_consumer_key", c.ck)
	params.Add("oauth_timestamp", strconv.Itoa(int(c.option.OauthTimestamp.Unix())))
	nonce := make([]byte, 16)
	rand.Read(nonce)
	sha1Nonce := fmt.Sprintf("%x", sha1.Sum(nonce))
	params.Add("oauth_nonce", sha1Nonce)
	params.Add("oauth_signature_method", HashAlgorithm)
	var keys []string
	for k, _ := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var paramStrs []string
	for _, key := range keys {
		paramStrs = append(paramStrs, fmt.Sprintf("%s=%s", key, params.Get(key)))
	}
	paramStr := strings.Join(paramStrs, "&")
	params.Add("oauth_signature", c.oauthSign(method, urlStr, paramStr))
	return params.Encode()
}

func (c *Client) oauthSign(method, endpoint, params string) string {
	signingKey := c.cs
	if c.option.Version != "v1" || c.option.Version != "v2" {
		signingKey = signingKey + "&"
	}

	a := strings.Join([]string{method, url.QueryEscape(endpoint), url.QueryEscape(params)}, "&")
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(a))
	signatureBytes := mac.Sum(nil)
	return base64.StdEncoding.EncodeToString(signatureBytes)
}

func (c *Client) request(method, endpoint string, params url.Values, data io.Reader) (io.ReadCloser, error) {
	urlstr := c.storeURL.String() + endpoint

	body := data
	if c.storeURL.Scheme == "https" {
		urlstr += "?" + c.basicAuth(params)
	} else {
		urlstr += "?" + c.oauth(method, urlstr, params)
	}
	fmt.Println(body)
	switch method {
	case http.MethodPost, http.MethodPut:
	case http.MethodDelete, http.MethodGet, http.MethodOptions:
	default:
		return nil, fmt.Errorf("Method is not recognised: %s", method)
	}
	req, err := http.NewRequest(method, urlstr, body)
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		return nil, err
	}
	resp, err := c.rawClient.Do(req)
	if err != nil {
		return nil, err
	}
	if (resp.StatusCode != http.StatusOK) && (resp.StatusCode != http.StatusCreated) {
		return nil, fmt.Errorf("Request failed: %s", resp.Status)
	}
	return resp.Body, nil
}

func (c *Client) Post(endpoint string, data io.Reader) (io.ReadCloser, error) {
	return c.request("POST", endpoint, nil, data)
}

func (c *Client) Put(endpoint string, data io.Reader) (io.ReadCloser, error) {
	return c.request("PUT", endpoint, nil, data)
}

func (c *Client) Get(endpoint string, params url.Values) (io.ReadCloser, error) {
	return c.request("GET", endpoint, params, nil)
}

func (c *Client) Delete(endpoint string, params url.Values) (io.ReadCloser, error) {
	return c.request("POST", endpoint, params, nil)
}

func (c *Client) Options(endpoint string) (io.ReadCloser, error) {
	return c.request("OPTIONS", endpoint, nil, nil)
}
