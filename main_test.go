package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/labstack/echo"
	"github.com/labstack/gommon/log"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/x/network/connstring"
	"github.com/nkonev/blog-store/client"
	"github.com/nkonev/blog-store/client/mocks"
	"github.com/nkonev/blog-store/handlers"
	"github.com/oliveagle/jsonpath"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/dig"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	test "net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
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
	initViper()

	log.Info("Set up")
	mongoUrl := getMongoUrl()
	client, err := mongo.NewClient(mongoUrl)
	if err != nil {
		log.Panicf("Error during create mongo client: %v", err)
	}
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
	err = client.Connect(ctx)
	if err != nil {
		log.Panicf("Error during connect: %v", err)
	}

	uri, err := connstring.Parse(mongoUrl)

	err = client.Database(uri.Database).Drop(context.Background())
	if err != nil {
		log.Panicf("Error during dropping database: '%v'", err)
	}
	log.Infof("Mongo database '%v' successfully dropped", uri.Database)

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
	container.Provide(configureMinio)
	container.Provide(configureHandler)
	container.Provide(configureEcho)
	container.Provide(configureMigrate)
	container.Provide(configureAuthMiddleware)
	container.Invoke(runMigrate)

	return container
}

func StringToReadCloser(s string) io.ReadCloser {
	return ioutil.NopCloser(bytes.NewReader([]byte(s)))
}

func TestLs(t *testing.T) {
	container := setUpContainerForIntegrationTests()
	mockClient := &mocks.RestClient{}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{StatusCode: 200, Body: StringToReadCloser(`{"id": 1, "login": "nikita k"}`)}, nil)
	container.Provide(func() client.RestClient {
		return mockClient
	})

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/ls", nil, e, "sess-cookie-1")
		assert.Equal(t, http.StatusOK, c)
		assert.NotEmpty(t, b)
		log.Infof("Got body: %v", b)
	})
}

func TestStaticIndex(t *testing.T) {

	container := setUpContainerForIntegrationTests()
	container.Provide(func() client.RestClient {
		return &mocks.RestClient{}
	})

	runTest(container, func(e *echo.Echo) {
		c, _, _ := request("GET", "/index.html", nil, e, "")
		assert.Equal(t, http.StatusMovedPermanently, c)
	})
}

func TestStaticRoot(t *testing.T) {

	container := setUpContainerForIntegrationTests()
	container.Provide(func() client.RestClient {
		return &mocks.RestClient{}
	})

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/", nil, e, "")
		assert.Equal(t, http.StatusOK, c)
		assert.Contains(t, b, "app-container")
	})
}

func TestStaticAssets(t *testing.T) {

	container := setUpContainerForIntegrationTests()
	container.Provide(func() client.RestClient {
		return &mocks.RestClient{}
	})

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

func TestUploadLs(t *testing.T) {
	container := setUpContainerForIntegrationTests()
	mockClient := &mocks.RestClient{}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{StatusCode: 200, Body: StringToReadCloser(`{"id": 1, "login": "nikita k"}`)}, nil)
	container.Provide(func() client.RestClient {
		return mockClient
	})

	runTest(container, func(e *echo.Echo) {
		path := "docker-compose.yml"
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
				echo.HeaderContentType: {"application/json"},
				echo.HeaderCookie:      []string{SESSION_COOKIE + "=" + "sessionCookie"},
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

func TestUploadDownloadDelete(t *testing.T) {
	container := setUpContainerForIntegrationTests()
	mockClient := &mocks.RestClient{}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{StatusCode: 200, Body: StringToReadCloser(`{"id": 1, "login": "nikita k"}`)}, nil)
	container.Provide(func() client.RestClient {
		return mockClient
	})

	runTest(container, func(e *echo.Echo) {
		path := "docker-compose.yml"
		fileName := "del_" + uuid.NewV4().String() + ".yml"
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
			req := test.NewRequest("GET", "/download/"+fileName, nil)
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
			assert.True(t, strings.Index(rec.Body.String(), "# This file used for both developer and demo purposes") == 0)
		}

		{
			req := test.NewRequest("DELETE", "/delete/"+fileName, nil)
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/download/"+fileName, nil)
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
	mockClient := &mocks.RestClient{}
	mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{StatusCode: 200, Body: StringToReadCloser(`{"id": 1, "login": "nikita k"}`)}, nil)
	container.Provide(func() client.RestClient {
		return mockClient
	})

	runTest(container, func(e *echo.Echo) {
		path := "docker-compose.yml"
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
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/download/"+fileNameNew, nil)
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			log.Infof("Got body: %v", rec.Body.String())
			assert.True(t, strings.Index(rec.Body.String(), "# This file used for both developer and demo purposes") == 0)
		}

	})
}
