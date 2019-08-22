package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"github.com/nkonev/blog-storage/client"
	"github.com/nkonev/blog-storage/handlers"
	"github.com/nkonev/blog-storage/utils"
	"github.com/oliveagle/jsonpath"
	"github.com/satori/go.uuid"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/dig"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	test "net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	setup()
	retCode := m.Run()
	shutdown()
	os.Exit(retCode)
}

func shutdown() {
	log.Info("Shutting down")
}

func setup() {
	utils.InitViper("./config-dev/config.yml")

	log.Info("Set up")
	utils.DropMongo()

	mc := configureMinio()
	infos, err := mc.ListBuckets()
	if err != nil {
		log.Panicf("Error during listing buckets: %v", err)
	}
	for _, b := range infos {
		// Create a done channel.
		doneCh := make(chan struct{})
		defer close(doneCh)
		// Recurively list all objects in 'mytestbucket'
		recursive := true
		log.Infof("Listing bucket '%v':", b.Name)
		for objInfo := range mc.ListObjects(b.Name, "", recursive, doneCh) {
			log.Infof("Object '%v'", objInfo.Key)
			err := mc.RemoveObject(b.Name, objInfo.Key)
			if err != nil {
				log.Panicf("Error during dropping object %v: %v", objInfo.Key, err)
			}
		}

		err := mc.RemoveBucket(b.Name)
		if err != nil {
			log.Panicf("Error during dropping bucket %v: %v", b.Name, err)
		}
	}
}

func request(method, path string, body io.Reader, e *echo.Echo, sessionCookie string) (int, string, http.Header) {
	req := test.NewRequest(method, path, body)
	Header := map[string][]string{
		echo.HeaderContentType: {"application/json"},
		echo.HeaderCookie:      []string{SESSION_COOKIE + "=" + sessionCookie},
	}
	req.Header = Header
	rec := test.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String(), rec.HeaderMap
}

func runTest(container *dig.Container, test func(e *echo.Echo)) {

	if err := container.Invoke(func(e *echo.Echo) {
		defer e.Close()

		test(e)
	}); err != nil {
		panic(err)
	}
}

func setUpContainerForIntegrationTests() *dig.Container {
	container := dig.New()
	container.Provide(configureMongo)
	container.Provide(configureMinio)
	container.Provide(configureHandler)
	container.Provide(configureEcho)
	container.Provide(configureMigrate)
	container.Provide(configureAuthMiddleware)
	container.Invoke(runMigrate)

	return container
}

func stringToReadCloser(s string) io.ReadCloser {
	return ioutil.NopCloser(bytes.NewReader([]byte(s)))
}

func TestLs(t *testing.T) {
	container := setUpContainerForIntegrationTests()
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container.Provide(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/ls", nil, e, "sess-cookie-1")
		assert.Equal(t, http.StatusOK, c)
		assert.NotEmpty(t, b)
		log.Infof("Got body: %v", b)
	})
}

func TestUnauthorizedWithoutCookie(t *testing.T) {
	container := setUpContainerForIntegrationTests()
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container.Provide(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		req := test.NewRequest("GET", "/ls", nil)
		rec := test.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.NotEmpty(t, rec.Body.String())
		log.Infof("Got body: %v", rec.Body.String())
	})
}

func TestUnauthorizedByServer(t *testing.T) {
	container := setUpContainerForIntegrationTests()
	testServer := makeFailAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container.Provide(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/ls", nil, e, "sess-cookie-1")
		assert.Equal(t, http.StatusUnauthorized, c)
		assert.NotEmpty(t, b)
		log.Infof("Got body: %v", b)
	})
}

func TestStaticIndex(t *testing.T) {

	container := setUpContainerForIntegrationTests()
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container.Provide(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		c, _, _ := request("GET", "/index.html", nil, e, "")
		assert.Equal(t, http.StatusMovedPermanently, c)
	})
}

func TestStaticRoot(t *testing.T) {

	container := setUpContainerForIntegrationTests()
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container.Provide(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/", nil, e, "")
		assert.Equal(t, http.StatusOK, c)
		assert.Contains(t, b, "app-container")
	})
}

func TestStaticAssets(t *testing.T) {

	container := setUpContainerForIntegrationTests()
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container.Provide(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/test-assets/main.js", nil, e, "")
		assert.Equal(t, http.StatusOK, c)
		assert.Equal(t, `console.log("Hello world");`, b)
	})
}

func getMultipart(bytea []byte, filename string) (*bytes.Buffer, string) {

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(handlers.FormFile, filename)
	if err != nil {
		log.Panicf("Error during creating form file")
	}

	_, err = io.Copy(part, bytes.NewReader(bytea))
	if err != nil {
		log.Panicf("Error during copy")
	}

	err = writer.Close()
	if err != nil {
		log.Panicf("Error during closing writer")
	}
	return body, writer.FormDataContentType()
}

func getBytea(path string) []byte {
	dat, err := ioutil.ReadFile(path)
	if err != nil {
		log.Panicf("Error during reading file")
	}
	return dat
}

func makeOkAuthServer() *test.Server {
	testServer := test.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(200)
		res.Write([]byte(`{"id": 1, "login": "nikita k"}`))
	}))
	return testServer
}

func makeFailAuthServer() *test.Server {
	testServer := test.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(401)
		res.Write([]byte(`{"status":401,"error":"Unauthorized","message":"Доступ запрещен","timeStamp":"Mon Feb 11 00:14:12 MSK 2019","validationErrors":[]}`))

	}))
	return testServer
}

func TestUploadLs(t *testing.T) {
	container := setUpContainerForIntegrationTests()

	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container.Provide(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		path := "test-file.yml"
		fileName := "ls_" + uuid.NewV4().String() + ".yml"
		{
			dat := getBytea(path)

			body, contentType := getMultipart(dat, fileName)

			req := test.NewRequest("POST", "/upload", body)
			headers := map[string][]string{
				echo.HeaderContentType: {contentType},
				echo.HeaderCookie:      []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/ls", nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())

			var arr = jsonPathHelper(rec.Body.String(), "$.files[?(@.filename =~ /(?i).*ls.*/)].filename").([]interface{})
			assert.Equal(t, fileName, arr[0])
		}
	})
}

func jsonPathHelper(in, jsonPath string) interface{} {
	var jsonData interface{}
	err := json.Unmarshal([]byte(in), &jsonData)
	if err != nil {
		log.Panicf("Error during unmarshall: %v", err)
	}

	res, err := jsonpath.JsonPathLookup(jsonData, jsonPath)
	if err != nil {
		log.Panicf("Error during requesting jsonpath: %v", err)
	}
	return res
}

// DownloadFile will download a url to a local file. It's efficient because it will
// write as it downloads and not load the whole file into memory.
func DownloadFile(url string) (string, error) {

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var b bytes.Buffer
	foo := bufio.NewWriter(&b)
	// Write the body to file
	_, err = io.Copy(foo, resp.Body)
	if err != nil {
		return "", err
	}
	err = foo.Flush()
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

func TestUploadDownloadDelete(t *testing.T) {
	container := setUpContainerForIntegrationTests()
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container.Provide(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		path := "test-file.yml"
		fileName := "del_" + uuid.NewV4().String() + "test+file?.yml"
		fileNameEncoded := url.PathEscape(fileName)
		{
			dat := getBytea(path)

			body, contentType := getMultipart(dat, fileName)

			req := test.NewRequest("POST", "/upload", body)
			headers := map[string][]string{
				echo.HeaderContentType: {contentType},
				echo.HeaderCookie:      []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/download/"+fileNameEncoded, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusTemporaryRedirect, rec.Code)
			assert.True(t, "927" == rec.Header().Get(echo.HeaderContentLength))
			assert.Equal(t, "application/octet-stream", rec.Header().Get(echo.HeaderContentType))
			body, err := DownloadFile(rec.Header().Get("Location"))
			assert.Nil(t, err)
			assert.NotEmpty(t, body)
			log.Infof("Got body: %v", body)
			assert.True(t, strings.Index(body, "# This file used for both developer and demo purposes") == 0)
		}

		{
			req := test.NewRequest("DELETE", "/delete/"+fileNameEncoded, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/download/"+fileNameEncoded, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusNotFound, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			var str = jsonPathHelper(rec.Body.String(), "$.status").(string)
			assert.Equal(t, "stat fail", str)
		}
	})
}

func TestUploadMove(t *testing.T) {
	container := setUpContainerForIntegrationTests()
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container.Provide(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		path := "test-file.yml"
		oldFileName := "pre_mv_" + uuid.NewV4().String() + ".yml"
		fileNameNew := "mv_" + uuid.NewV4().String() + ".yml"
		{
			dat := getBytea(path)

			body, contentType := getMultipart(dat, oldFileName)

			req := test.NewRequest("POST", "/upload", body)
			headers := map[string][]string{
				echo.HeaderContentType: {contentType},
				echo.HeaderCookie:      []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("POST", "/move/"+oldFileName+"/"+fileNameNew, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/download/"+fileNameNew, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusTemporaryRedirect, rec.Code)
			body, err := DownloadFile(rec.Header().Get("Location"))
			assert.Nil(t, err)
			assert.NotEmpty(t, body)
			log.Infof("Got body: %v", body)
			assert.True(t, strings.Index(body, "# This file used for both developer and demo purposes") == 0)
		}

	})
}

func TestUploadPublish(t *testing.T) {
	container := setUpContainerForIntegrationTests()
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container.Provide(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		path := "test-file.yml"
		fileName := "publish_" + uuid.NewV4().String() + ".yml"
		{
			dat := getBytea(path)

			body, contentType := getMultipart(dat, fileName)

			req := test.NewRequest("POST", "/upload", body)
			headers := map[string][]string{
				echo.HeaderContentType: {contentType},
				echo.HeaderCookie:      []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/public/user1/"+fileName, nil)
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusNotFound, rec.Code)
		}

		{
			req := test.NewRequest("PUT", "/publish/"+fileName, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/public/user1/"+fileName, nil)
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			body, err := DownloadFile(rec.Header().Get("Location"))
			assert.Nil(t, err)
			assert.NotEmpty(t, body)
			log.Infof("Got body: %v", body)
			assert.True(t, strings.Index(body, "# This file used for both developer and demo purposes") == 0)
		}

		{
			req := test.NewRequest("DELETE", "/publish/"+fileName, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/public/user1/"+fileName, nil)
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusNotFound, rec.Code)
		}

	})
}
