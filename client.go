package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

type fault struct {
	descr          string
	dn             string
	lastTransition time.Time
	lc             string
}

// client is an ACI API client
type client struct {
	host                    string
	usr                     string
	pwd                     string
	url                     url.URL
	httpClient              *http.Client
	lastRefresh             time.Time
	lastSubscriptionRefresh time.Time
	clearDelay              int
	refreshTimeout          int
	subscriptionID          string
	faults                  map[string]fault
}

// newACIClient configures and returns a new ACI API client
func newACIClient(a args) *client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	cookieJar, _ := cookiejar.New(nil)
	httpClient := &http.Client{
		Timeout:   time.Duration(a.HTTPTimeout) * time.Second,
		Transport: tr,
		Jar:       cookieJar,
	}
	return &client{
		httpClient: httpClient,
		host:       a.APIC,
		usr:        a.Usr,
		pwd:        a.Pwd,
		url:        url.URL{Scheme: "https", Host: a.APIC},
		clearDelay: a.ClearDelay,
		faults:     make(map[string]fault),
	}
}

func (c *client) newFault(record gjson.Result) error {
	ageStr := record.Get("lastTransition").Str
	age, err := time.Parse(time.RFC3339, ageStr)
	if err != nil {
		return err
	}

	f := fault{
		dn:             record.Get("dn").Str,
		descr:          record.Get("descr").Str,
		lc:             record.Get("lc").Str,
		lastTransition: age,
	}
	c.faults[f.dn] = f
	return nil
}

// token gets the login token from the current cookie
func (c *client) token() string {
	for _, cookie := range c.httpClient.Jar.Cookies(&c.url) {
		if cookie.Name == "APIC-cookie" {
			return cookie.Value
		}
	}
	return ""
}

func getNodeDn(dn string) string {
	return strings.Join(strings.Split(dn, "/")[:3], "/")
}

// login authenticates the fabric
func (c *client) login() error {
	log.Info().Msgf("Loging in to %s", c.host)
	data := json{}.set("aaaUser.attributes", map[string]string{
		"name": c.usr,
		"pwd":  c.pwd,
	}).str
	res, err := c.httpClient.Post(c.url.String()+"/api/aaaLogin.json",
		"application/json",
		strings.NewReader(data))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("HTTP status code: %d", res.StatusCode))
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	record := gjson.GetBytes(body, "imdata.0")
	errText := record.Get("error.attributes.text").Str
	if errText != "" {
		return errors.New(errText)
	}
	timeoutStr := record.Get("aaaLogin.attributes.refreshTimeoutSeconds").Str
	c.refreshTimeout, err = strconv.Atoi(timeoutStr)
	if err != nil {
		log.Error().Err(err).Msg("cannot convert refreshTimeoutSeconds")
		c.refreshTimeout = 600
	}

	c.lastRefresh = time.Now()
	return nil
}

// refresh refreshes the login token
func (c *client) refresh() error {
	log.Debug().Msg("Refreshing login token")
	res, err := c.httpClient.Get(c.url.String() + "/api/aaaRefresh")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	c.lastRefresh = time.Now()
	return nil
}

func (c *client) refreshLoop() error {
	for {
		elapsed := time.Since(c.lastRefresh)
		limit := time.Duration(c.refreshTimeout-30) * time.Second
		if elapsed > limit {
			if err := c.refresh(); err != nil {
				return err
			}
		}
		time.Sleep(1 * time.Second)
	}
}

// connectSocket connects the websocket
func (c *client) connectSocket() (*websocket.Conn, error) {
	log.Info().Msg("Connecting websocket")

	wsURL := url.URL{
		Scheme: "ws",
		Host:   c.host,
		Path:   "/socket" + c.token(),
	}

	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	ws, _, err := dialer.Dial(wsURL.String(), nil)
	return ws, err
}

// listenSocket listents for incoming websocket messages
func (c *client) listenSocket(ws *websocket.Conn) error {
	log.Info().Msg("Listening for incoming messages")
	defer ws.Close()
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		if gjson.ValidBytes(msg) {
			record := gjson.GetBytes(msg, "imdata.0.faultInst.attributes")
			if err := c.newFault(record); err != nil {
				log.Warn().Err(err).Msgf("Error reading fault record")
			}
		} else {
			log.Warn().Msgf("Non-JSON msg rcvd: %s\n", msg)
		}
	}
}

// subscribe subscribes to REP faults
func (c *client) subscribe() error {
	log.Info().Msg("Susbscribing to REP faults")
	filter := fmt.Sprintf("or(%s,%s)",
		`eq(faultInst.code,"F3013")`,
		`eq(faultInst.code,"F3014")`)

	queryValues := url.Values{}
	queryValues.Add("query-target-filter", filter)
	queryValues.Add("subscription", "yes")

	u := url.URL{
		Scheme:   c.url.Scheme,
		Host:     c.url.Host,
		Path:     "/api/class/faultInst.json",
		RawQuery: queryValues.Encode(),
	}

	res, err := c.httpClient.Get(u.String())
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	jsonRes := gjson.ParseBytes(body)
	if errStr := jsonRes.Get("imdata.0.error.attributes.text").Str; errStr != "" {
		return errors.New(errStr)
	}
	subscriptionID := jsonRes.Get("subscriptionId").Str
	if subscriptionID == "" {
		return errors.New("no subscription ID in reply")
	}
	c.subscriptionID = subscriptionID
	c.lastSubscriptionRefresh = time.Now()
	for _, record := range jsonRes.Get("imdata.#.faultInst.attributes").Array() {
		if err := c.newFault(record); err != nil {
			return err
		}
	}
	return nil
}

func (c *client) refreshSubscription() error {
	log.Debug().Msg("Refreshing subscription")
	u := fmt.Sprintf("%s/api/subscriptionRefresh.json?id=%s",
		c.url.String(),
		c.subscriptionID,
	)
	res, err := c.httpClient.Get(u)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	errStr := gjson.GetBytes(body, "imdata.0.error.attributes.text").Str
	if errStr != "" {
		return errors.New(errStr)
	}
	c.lastSubscriptionRefresh = time.Now()
	return nil
}

func (c *client) subscriptionRefreshLoop() error {
	log.Info().Msg("Starting subscription refresh loop")
	for {
		elapsed := time.Since(c.lastSubscriptionRefresh)
		limit := 30 * time.Second
		if elapsed > limit {
			if err := c.refreshSubscription(); err != nil {
				return err
			}
		}
		time.Sleep(1 * time.Second)
	}
}

func (c *client) clearNode(dn string) error {
	if dn == "" {
		return errors.New("Empty DN")
	}
	if !strings.HasPrefix(dn, "topology/pod-") {
		return fmt.Errorf("Unexpected DN format: %s", dn)
	}
	nodeDn := getNodeDn(dn)
	log.Debug().Msgf("Clearing node for fault: %s", dn)
	log.Info().Msgf("Clearing node %s", nodeDn)

	lsubj := fmt.Sprintf("%s/sys/action/lsubj-[%s]", nodeDn, nodeDn)

	data := json{}.
		set("actionLSubj", json{}.
			set("attributes", map[string]string{
				"dn":  lsubj,
				"oDn": nodeDn,
			}).
			set("children.0.topSystemClearEpLTask", json{}.
				set("attributes", map[string]string{
					"dn":      fmt.Sprintf("%s/topSystemClearEpLTask", lsubj),
					"adminSt": "start",
				}).
				set("children", []string{}))).str
	u := url.URL{
		Scheme: c.url.Scheme,
		Host:   c.url.Host,
		Path:   fmt.Sprintf("/api/node/mo/%s/sys/action.json", nodeDn),
	}
	res, err := c.httpClient.Post(u.String(),
		"application/json",
		strings.NewReader(data))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	errMsg := gjson.GetBytes(body, "imdata.0.error.attributes.text").Str
	if errMsg != "" {
		return errors.New(errMsg)
	}
	return nil
}

func (c *client) clearNodeLoop() {
	delay := time.Duration(c.clearDelay) * time.Second
	for {
		for dn, fault := range c.faults {
			elapsed := time.Since(fault.lastTransition)
			if elapsed > delay && fault.lc == "raised" {
				err := c.clearNode(dn)
				if err != nil {
					log.Error().Err(err).Msg("Clearing node")
				}
				delete(c.faults, dn)
			}
		}
		time.Sleep(time.Second)
	}
}
