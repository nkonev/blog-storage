package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/labstack/echo/v4"
	"github.com/nkonev/blog-storage/client"
	"github.com/nkonev/blog-storage/data/repository"
	"github.com/nkonev/blog-storage/handlers"
	. "github.com/nkonev/blog-storage/logger"
	"github.com/nkonev/blog-storage/utils"
	"github.com/oliveagle/jsonpath"
	uuid "github.com/satori/go.uuid"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
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

func TestMain(m *testing.M) {
	setup()
	retCode := m.Run()
	shutdown()
	os.Exit(retCode)
}

func shutdown() {
	Logger.Info("Shutting down")
}

func setup() {
	utils.InitViper("./config-dev/config.yml")

	Logger.Info("Set up")
	utils.DropMongo()

	mc := configureMinio()
	infos, err := mc.ListBuckets()
	if err != nil {
		Logger.Panicf("Error during listing buckets: %v", err)
	}
	for _, b := range infos {
		// Create a done channel.
		doneCh := make(chan struct{})
		defer close(doneCh)
		// Recurively list all objects in 'mytestbucket'
		recursive := true
		Logger.Infof("Listing bucket '%v':", b.Name)
		for objInfo := range mc.ListObjects(b.Name, "", recursive, doneCh) {
			Logger.Infof("Object '%v'", objInfo.Key)
			err := mc.RemoveObject(b.Name, objInfo.Key)
			if err != nil {
				Logger.Panicf("Error during dropping object %v: %v", objInfo.Key, err)
			}
		}

		err := mc.RemoveBucket(b.Name)
		if err != nil {
			Logger.Panicf("Error during dropping bucket %v: %v", b.Name, err)
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

func runTest(container fx.Option, test func(e *echo.Echo)) {

	app := fx.New(
		container,
		fx.Invoke(runMigrate, runEcho2(test)),
	)
	Logger.Infof("Running")
	stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := app.Start(stopCtx); err != nil {
		panic(err)
	}

	Logger.Infof("Stopping")
	stopCtx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel2()
	if err := app.Stop(stopCtx2); err != nil {
		Logger.Fatal(err)
	}
}

func runEcho2(test func(e *echo.Echo)) func(e *echo.Echo) {
	return func(e *echo.Echo) {
		test(e)
		Logger.Infof("Test finished")
	}
}

func setUpContainerForIntegrationTests(additional ...interface{}) fx.Option {
	var arr []interface{}
	arr = append(arr, configureMongo, configureMinio,
		repository.NewGlogalIdRepository,
		repository.NewUserFileRepository,
		repository.NewLimitsRepository,
		handlers.NewFsHandler, configureEcho, configureMigrate,
		configureAuthMiddleware, configureStaticMiddleware, configureTransactionMiddleware)
	arr = append(arr, additional...)
	return fx.Provide(arr...)

}

func stringToReadCloser(s string) io.ReadCloser {
	return ioutil.NopCloser(bytes.NewReader([]byte(s)))
}

func TestLs(t *testing.T) {
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)

	container := setUpContainerForIntegrationTests(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/ls", nil, e, "sess-cookie-1")
		assert.Equal(t, http.StatusOK, c)
		assert.NotEmpty(t, b)
		Logger.Infof("Got body: %v", b)
	})
}

func TestUnauthorizedWithoutCookie(t *testing.T) {
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container := setUpContainerForIntegrationTests(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		req := test.NewRequest("GET", "/ls", nil)
		rec := test.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.NotEmpty(t, rec.Body.String())
		Logger.Infof("Got body: %v", rec.Body.String())
	})
}

func TestUnauthorizedByServer(t *testing.T) {
	testServer := makeFailAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container := setUpContainerForIntegrationTests(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/ls", nil, e, "sess-cookie-1")
		assert.Equal(t, http.StatusUnauthorized, c)
		assert.NotEmpty(t, b)
		Logger.Infof("Got body: %v", b)
	})
}

func TestStaticIndex(t *testing.T) {

	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container := setUpContainerForIntegrationTests(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		c, _, _ := request("GET", "/index.html", nil, e, "")
		assert.Equal(t, http.StatusMovedPermanently, c)
	})
}

func TestStaticRoot(t *testing.T) {

	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container := setUpContainerForIntegrationTests(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/", nil, e, "")
		assert.Equal(t, http.StatusOK, c)
		assert.Contains(t, b, "app-container")
	})
}

func TestStaticAssets(t *testing.T) {

	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container := setUpContainerForIntegrationTests(client.NewRestClient)

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
		Logger.Panicf("Error during creating form file")
	}

	_, err = io.Copy(part, bytes.NewReader(bytea))
	if err != nil {
		Logger.Panicf("Error during copy")
	}

	err = writer.Close()
	if err != nil {
		Logger.Panicf("Error during closing writer")
	}
	return body, writer.FormDataContentType()
}

func getBytea(path string) []byte {
	dat, err := ioutil.ReadFile(path)
	if err != nil {
		Logger.Panicf("Error during reading file")
	}
	return dat
}

func TestUploadLs(t *testing.T) {

	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container := setUpContainerForIntegrationTests(client.NewRestClient)

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
			Logger.Infof("Got body: %v", rec.Body.String())
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
			Logger.Infof("Got body: %v", rec.Body.String())

			var arr = jsonPathHelper(rec.Body.String(), "$.files[?(@.filename =~ /(?i).*ls.*/)].filename").([]interface{})
			assert.Equal(t, fileName, arr[0])
		}
	})
}

func jsonPathHelper(in, jsonPath string) interface{} {
	var jsonData interface{}
	err := json.Unmarshal([]byte(in), &jsonData)
	if err != nil {
		Logger.Panicf("Error during unmarshall: %v", err)
	}

	res, err := jsonpath.JsonPathLookup(jsonData, jsonPath)
	if err != nil {
		Logger.Panicf("Error during requesting jsonpath: %v", err)
	}
	return res
}

func TestUploadDownloadDelete(t *testing.T) {
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container := setUpContainerForIntegrationTests(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		path := "test-file.yml"
		fileName := "del_" + uuid.NewV4().String() + "test+file?.yml"
		var fileId = "empty"
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
			fileId = getFileIdFromResp(rec, t)
			Logger.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/download/"+fileId, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.True(t, "927" == rec.Header().Get(echo.HeaderContentLength))
			assert.Equal(t, "application/octet-stream", rec.Header().Get(echo.HeaderContentType))
			assert.NotEmpty(t, rec.Body.String())
			Logger.Infof("Got body: %v", rec.Body.String())
			assert.True(t, strings.Index(rec.Body.String(), "# This file used for both developer and demo purposes") == 0)
		}

		{
			req := test.NewRequest("DELETE", "/delete/"+fileId, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			Logger.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/download/"+fileId, nil)
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

func getFileIdFromResp(rec *test.ResponseRecorder, t *testing.T) string {
	var fileId string
	var resp = &utils.H{}
	err := json.Unmarshal([]byte(rec.Body.Bytes()), &resp)
	assert.Nil(t, err)
	fileId = (*resp)["id"].(string)
	return fileId
}

func TestUploadRename(t *testing.T) {
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container := setUpContainerForIntegrationTests(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		path := "test-file.yml"
		oldFileName := "pre_mv_" + uuid.NewV4().String() + ".yml"
		fileNameNew := "mv_" + uuid.NewV4().String() + ".yml"
		var fileId string
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
			Logger.Infof("Got body: %v", rec.Body.String())
			fileId = getFileIdFromResp(rec, t)
		}

		{
			req := test.NewRequest("POST", "/rename/"+fileId, strings.NewReader(`{"newname": "`+fileNameNew+`"}`))
			headers := map[string][]string{
				echo.HeaderContentType: {"application/json"},
				echo.HeaderCookie:      []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			Logger.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/download/"+fileId, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			Logger.Infof("Got body: %v", rec.Body.String())
			assert.True(t, strings.Index(rec.Body.String(), "# This file used for both developer and demo purposes") == 0)
		}

	})
}

func TestUploadPublish(t *testing.T) {
	testServer := makeOkAuthServer()
	defer func() { testServer.Close() }()
	viper.Set(AUTH_URL, testServer.URL)
	container := setUpContainerForIntegrationTests(client.NewRestClient)

	runTest(container, func(e *echo.Echo) {
		path := "test-file.yml"
		fileName := "publish_" + uuid.NewV4().String() + ".yml"
		var fileId string
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
			Logger.Infof("Got body: %v", rec.Body.String())
			fileId = getFileIdFromResp(rec, t)
		}

		{
			req := test.NewRequest("GET", "/public/user1/"+fileId, nil)
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusNotFound, rec.Code)
		}

		{
			req := test.NewRequest("PUT", "/publish/"+fileId, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			Logger.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/public/user1/"+fileId, nil)
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			Logger.Infof("Got body: %v", rec.Body.String())
			assert.True(t, strings.Index(rec.Body.String(), "# This file used for both developer and demo purposes") == 0)
		}

		{
			req := test.NewRequest("DELETE", "/publish/"+fileId, nil)
			headers := map[string][]string{
				echo.HeaderCookie: []string{SESSION_COOKIE + "=" + "sessionCookie"},
			}
			req.Header = headers
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.NotEmpty(t, rec.Body.String())
			Logger.Infof("Got body: %v", rec.Body.String())
		}

		{
			req := test.NewRequest("GET", "/public/user1/"+fileId, nil)
			rec := test.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusNotFound, rec.Code)
		}

	})
}
