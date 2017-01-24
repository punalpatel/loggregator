package main_test

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	syslog_drain_binder "syslog_drain_binder"
)

var _ = Describe("CloudControllerPoller", func() {
	var _ = Describe("GetSyslogDrainURLs", func() {
		var (
			testServer          *httptest.Server
			fakeCloudController fakeCC
			baseURL             string
		)

		BeforeEach(func() {
			fakeCloudController = fakeCC{}

			testServer = httptest.NewServer(
				http.HandlerFunc(fakeCloudController.ServeHTTP),
			)
			baseURL = "http://" + testServer.Listener.Addr().String()
		})

		AfterEach(func() {
			testServer.Close()
		})

		It("connects to the correct endpoint with basic authentication and the expected parameters", func() {
			syslog_drain_binder.Poll(baseURL, "user", "pass", 2)
			Expect(fakeCloudController.servedRoute).To(Equal("/v2/syslog_drain_urls"))
			Expect(fakeCloudController.username).To(Equal("user"))
			Expect(fakeCloudController.password).To(Equal("pass"))

			Expect(fakeCloudController.queryParams).To(HaveKeyWithValue("batch_size", []string{"2"}))
		})

		It("processes all pages into a single result with batch_size 2", func() {
			drainUrls, err := syslog_drain_binder.Poll(baseURL, "user", "pass", 2)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeCloudController.requestCount).To(Equal(6))

			for _, entry := range appDrains {
				Expect(drainUrls).To(HaveKeyWithValue(entry.appID, entry.urls))
			}
		})

		It("processes all pages into a single result with batch_size 3", func() {
			drainUrls, err := syslog_drain_binder.Poll(baseURL, "user", "pass", 3)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeCloudController.requestCount).To(Equal(5))

			for _, entry := range appDrains {
				Expect(drainUrls).To(HaveKeyWithValue(entry.appID, entry.urls))
			}
		})

		Context("when CC becomes unreachable before finishing", func() {
			BeforeEach(func() {
				fakeCloudController.failOn = 4
			})

			It("returns as much data as it has, and an error", func() {
				drainUrls, err := syslog_drain_binder.Poll(baseURL, "user", "pass", 2)
				Expect(err).To(HaveOccurred())

				Expect(fakeCloudController.requestCount).To(Equal(4))

				for i := 0; i < 8; i++ {
					entry := appDrains[i]
					Expect(drainUrls).To(HaveKeyWithValue(entry.appID, entry.urls))
				}

				for i := 8; i < 10; i++ {
					entry := appDrains[i]
					Expect(drainUrls).NotTo(HaveKeyWithValue(entry.appID, entry.urls))
				}
			})
		})

		Context("when connecting to a secure server with a self-signed certificate", func() {
			var secureTestServer *httptest.Server

			BeforeEach(func() {
				secureTestServer = httptest.NewUnstartedServer(http.HandlerFunc(fakeCloudController.ServeHTTP))
				secureTestServer.TLS = &tls.Config{
					CipherSuites: []uint16{
						tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
						tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					},
					MinVersion: tls.VersionTLS12,
				}
				secureTestServer.StartTLS()
				baseURL = "https://" + secureTestServer.Listener.Addr().String()
			})

			AfterEach(func() {
				secureTestServer.Close()
			})

			It("fails to connect if skipCertVerify is false", func() {
				_, err := syslog_drain_binder.Poll(baseURL, "user", "pass", 2)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("certificate signed by unknown authority"))
			})

			It("successfully connects if skipCertVerify is true", func() {
				_, err := syslog_drain_binder.Poll(baseURL, "user", "pass", 2, syslog_drain_binder.SkipCertVerify(true))
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with the cloud controller not responding", func() {
			var serverNotResponding net.Listener

			BeforeEach(func() {
				var err error
				serverNotResponding, err = net.Listen("tcp", ":0")
				Expect(err).ToNot(HaveOccurred())
			})

			It("times out by default", func() {
				unpatch := patchDefaultTimeout(10 * time.Millisecond)
				defer unpatch()
				baseURL = "http://" + serverNotResponding.Addr().String()

				errs := make(chan error)
				go func() {
					_, err := syslog_drain_binder.Poll(baseURL, "user", "pass", 2)
					errs <- err
				}()

				Eventually(errs, 100*time.Millisecond).Should(Receive())
			})

			It("times out with explicit timeout", func() {
				baseURL = "http://" + serverNotResponding.Addr().String()

				errs := make(chan error)
				go func() {
					_, err := syslog_drain_binder.Poll(baseURL, "user", "pass", 2, syslog_drain_binder.Timeout(10*time.Millisecond))
					errs <- err
				}()

				Eventually(errs, 100*time.Millisecond).Should(Receive())
			})
		})
	})
})

func patchDefaultTimeout(t time.Duration) func() {
	orig := syslog_drain_binder.DefaultTimeout
	syslog_drain_binder.DefaultTimeout = t
	return func() {
		syslog_drain_binder.DefaultTimeout = orig
	}
}

type appEntry struct {
	appID syslog_drain_binder.AppID
	urls  []syslog_drain_binder.DrainURL
}

var appDrains = []appEntry{
	{appID: "app0", urls: []syslog_drain_binder.DrainURL{"urlA"}},
	{appID: "app1", urls: []syslog_drain_binder.DrainURL{"urlB"}},
	{appID: "app2", urls: []syslog_drain_binder.DrainURL{"urlA", "urlC"}},
	{appID: "app3", urls: []syslog_drain_binder.DrainURL{"urlA", "urlD", "urlE"}},
	{appID: "app4", urls: []syslog_drain_binder.DrainURL{"urlA"}},
	{appID: "app5", urls: []syslog_drain_binder.DrainURL{"urlA"}},
	{appID: "app6", urls: []syslog_drain_binder.DrainURL{"urlA"}},
	{appID: "app7", urls: []syslog_drain_binder.DrainURL{"urlA"}},
	{appID: "app8", urls: []syslog_drain_binder.DrainURL{"urlA"}},
	{appID: "app9", urls: []syslog_drain_binder.DrainURL{"urlA"}},
}

type fakeCC struct {
	servedRoute  string
	username     string
	password     string
	queryParams  url.Values
	requestCount int
	failOn       int
}

func (fake *fakeCC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if fake.failOn > 0 && fake.requestCount >= fake.failOn {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	fake.requestCount++
	fake.servedRoute = r.URL.Path
	fake.queryParams = r.URL.Query()

	auth := r.Header.Get("Authorization")
	parts := strings.Split(auth, " ")
	decodedBytes, _ := base64.StdEncoding.DecodeString(parts[1])
	creds := strings.Split(string(decodedBytes), ":")

	fake.username = creds[0]
	fake.password = creds[1]

	batchSize, _ := strconv.Atoi(fake.queryParams.Get("batch_size"))
	start, _ := strconv.Atoi(fake.queryParams.Get("next_id"))

	w.Write(buildResponse(start, start+batchSize))
}

func buildResponse(start int, end int) []byte {
	var r jsonResponse
	if start >= 10 {
		r = jsonResponse{
			Results: make(map[syslog_drain_binder.AppID][]syslog_drain_binder.DrainURL),
			NextId:  nil,
		}
	} else {
		r = jsonResponse{
			Results: make(map[syslog_drain_binder.AppID][]syslog_drain_binder.DrainURL),
			NextId:  &end,
		}

		for i := start; i < end && i < 10; i++ {
			r.Results[appDrains[i].appID] = appDrains[i].urls
		}
	}

	b, _ := json.Marshal(r)
	return b
}

type jsonResponse struct {
	Results map[syslog_drain_binder.AppID][]syslog_drain_binder.DrainURL `json:"results"`
	NextId  *int                                                         `json:"next_id"`
}
