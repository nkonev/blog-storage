package main

import (
	"context"
	"github.com/labstack/echo"
	"github.com/labstack/gommon/log"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/x/network/connstring"
	"github.com/stretchr/testify/assert"
	"go.uber.org/dig"
	"io"
	"net/http"
	test "net/http/httptest"
	"os"
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
		log.Panicf("Error during dropping database: %v", err)
	}
	log.Infof("Mongo database %v successfully dropped", uri.Database)
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
