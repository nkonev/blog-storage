package main

import (
	"bytes"
	"context"
	"github.com/labstack/echo"
	"github.com/labstack/gommon/log"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/x/network/connstring"
	"github.com/nkonev/blog-store/handlers"
	"github.com/stretchr/testify/assert"
	"go.uber.org/dig"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	test "net/http/httptest"
	"os"
	"path/filepath"
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
		echo.HeaderCookie:      []string{},
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

	container.Invoke(runMigrate)

	return container
}

func TestLs(t *testing.T) {
	container := setUpContainerForIntegrationTests()

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/ls", nil, e, "")
		assert.Equal(t, http.StatusOK, c)
		assert.NotEmpty(t, b)
		log.Infof("Got body: %v", b)
	})
}

func TestStaticIndex(t *testing.T) {

	container := setUpContainerForIntegrationTests()

	runTest(container, func(e *echo.Echo) {
		c, _, _ := request("GET", "/index.html", nil, e, "")
		assert.Equal(t, http.StatusMovedPermanently, c)
	})
}

func TestStaticRoot(t *testing.T) {

	container := setUpContainerForIntegrationTests()

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/", nil, e, "")
		assert.Equal(t, http.StatusOK, c)
		assert.Contains(t, b, "app-container")
	})
}

func TestStaticAssets(t *testing.T) {

	container := setUpContainerForIntegrationTests()

	runTest(container, func(e *echo.Echo) {
		c, b, _ := request("GET", "/test-assets/main.js", nil, e, "")
		assert.Equal(t, http.StatusOK, c)
		assert.Equal(t, `console.log("Hello world");`, b)
	})
}

func getMultipart(path string) (*bytes.Buffer, string) {
	dat, err := ioutil.ReadFile(path)
	if err != nil {
		log.Panicf("Error during reading file")
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(handlers.FormFile, filepath.Base(path))
	if err != nil {
		log.Panicf("Error during creating form file")
	}

	_, err = io.Copy(part, bytes.NewReader(dat))
	if err != nil {
		log.Panicf("Error during copy")
	}

	err = writer.Close()
	if err != nil {
		log.Panicf("Error during closing writer")
	}
	return body, writer.FormDataContentType()
}

func TestUploadDownload(t *testing.T) {
	container := setUpContainerForIntegrationTests()

	runTest(container, func(e *echo.Echo) {
		path := "./docker-compose.yml"

		body, contentType := getMultipart(path)

		req := test.NewRequest("POST", "/upload", body)
		headers := map[string][]string{
			echo.HeaderContentType: {contentType},
			echo.HeaderCookie:      []string{},
		}
		req.Header = headers
		rec := test.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.NotEmpty(t, rec.Body.String())
		log.Infof("Got body: %v", rec.Body.String())
	})
}
