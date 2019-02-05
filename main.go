package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gobuffalo/packr"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/labstack/gommon/log"
	"github.com/nkonev/blog-store/handlers"
	"github.com/nkonev/blog-store/utils"
	"github.com/spf13/viper"
	"go.uber.org/dig"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mongodb"
	"github.com/minio/minio-go"
	packr_migrate "github.com/nkonev/blog-store/packr"
)

func configureEcho(fsh *handlers.FsHandler, authMiddleware echo.MiddlewareFunc) *echo.Echo {
	bodyLimit := viper.GetString("server.body.limit")

	log.SetOutput(os.Stdout)

	static := packr.NewBox("./static")

	e := echo.New()

	e.Use(authMiddleware)

	e.Use(middleware.Logger())
	e.Use(middleware.Secure())
	e.Use(middleware.BodyLimit(bodyLimit))

	e.GET("/ls", fsh.LsHandler)
	e.POST("/upload", fsh.UploadHandler)
	e.GET("/download/:file", fsh.DownloadHandler)
	e.POST("/move/:from/:to", fsh.MoveHandler)
	e.DELETE("/delete/:file", fsh.DeleteHandler)

	e.Pre(getStaticMiddleware(static))

	return e
}

func initViper() {
	viper.SetConfigName("config")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("./config-dev")
	// call multiple times to add many search paths
	viper.SetEnvPrefix("BLOG_STORE")
	viper.AutomaticEnv()
	// Find and read the config file
	if err := viper.ReadInConfig(); err != nil { // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
}

func getStaticMiddleware(box packr.Box) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			reqUrl := c.Request().RequestURI
			if reqUrl == "/" || reqUrl == "/index.html" || reqUrl == "/favicon.ico" || strings.HasPrefix(reqUrl, "/build") || strings.HasPrefix(reqUrl, "/test-assets") {
				http.FileServer(box).
					ServeHTTP(c.Response().Writer, c.Request())
				return nil
			} else {
				return next(c)
			}
		}
	}
}

func checkUrlInWhitelist(whitelist []regexp.Regexp, uri string) bool {
	for _, regexp0 := range whitelist {
		if regexp0.MatchString(uri) {
			log.Infof("Skipping authentication for %v because it matches %v", uri, regexp0.String())
			return true
		}
	}
	return false
}

const SESSION_COOKIE = "SESSION"

func configureAuthMiddleware(httpClient *http.Client) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			whitelistStr := viper.GetStringSlice("auth.exclude")
			whitelist := utils.StringsToRegexpArray(whitelistStr)
			if checkUrlInWhitelist(whitelist, c.Request().RequestURI) {
				return next(c)
			}

			co, err := c.Request().Cookie(SESSION_COOKIE)
			if err != nil {
				log.Errorf("Error get '%v' cookie: %v", SESSION_COOKIE, err)
				return err
			}

			// check cookie
			req, err := http.NewRequest(
				"GET", "http://127.0.0.1:8080/api/profile", nil,
			)
			if err != nil {
				log.Errorf("Error during create request: %v", err)
				return err
			}

			req.AddCookie(co)
			resp, err := httpClient.Do(req)
			if err != nil {
				log.Errorf("Error during requesting auth backend: %v", err)
				return err
			}
			defer resp.Body.Close()

			// put user id, user name to context
			decoder := json.NewDecoder(resp.Body)
			var decodedResponse interface{}
			err = decoder.Decode(decodedResponse)
			if err != nil {
				log.Errorf("Error during decoding json: %v", err)
				return err
			}

			return next(c)
		}
	}
}

func main() {
	initViper()
	container := dig.New()
	container.Provide(configureMinio)
	container.Provide(configureHandler)
	container.Provide(configureEcho)
	container.Provide(configureMigrate)
	container.Provide(configureAuthMiddleware)
	container.Provide(configureHttpClient)
	container.Invoke(runMigrate)

	if echoErr := container.Invoke(runEcho); echoErr != nil {
		log.Fatalf("Error during invoke echo: %v", echoErr)
	}
	log.Infof("Exit program")
}

func configureHttpClient() *http.Client {
	tr := &http.Transport{
		MaxIdleConns:       viper.GetInt("http.idle.conns.max"),
		IdleConnTimeout:    viper.GetDuration("http.idle.connTimeout"),
		DisableCompression: viper.GetBool("http.disableCompression"),
	}
	client := &http.Client{Transport: tr}
	return client
}

func configureHandler(m *minio.Client) *handlers.FsHandler {
	return handlers.NewFsHandler(m)
}

func configureMigrate() *migrate.Migrate {
	box := packr.NewBox("./migrations")

	d, err := packr_migrate.WithInstance(&box)
	if err != nil {
		log.Panicf("Error during create migrator driver: %v", err)
	}

	m, err := migrate.NewWithSourceInstance(packr_migrate.PackrName, d, getMongoUrl())

	if err != nil {
		log.Panicf("Error during create migrator: %v", err)
	}
	return m
}

func getMongoUrl() string {
	return viper.GetString("mongo.migrations.databaseUrl")
}

func runMigrate(m *migrate.Migrate) {
	err := m.Up()
	if err != nil {
		if err.Error() == "no change" {
			log.Info("Migration(s) already applied")
		} else {
			log.Panicf("Error during applying migrations: %v", err)
		}
	}
	log.Info("Migration run successfully")
}

func configureMinio() *minio.Client {
	endpoint := viper.GetString("minio.endpoint")
	accessKeyID := viper.GetString("minio.accessKeyId")
	secretAccessKey := viper.GetString("minio.secretAccessKey")
	useSSL := false

	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, accessKeyID, secretAccessKey, useSSL)
	if err != nil {
		log.Fatal(err)
	}

	return minioClient
}

// rely on viper import and it's configured by
func runEcho(e *echo.Echo) {
	address := viper.GetString("server.address")
	shutdownTimeout := viper.GetDuration("server.shutdown.timeout")

	log.Info("Starting server...")
	// Start server in another goroutine
	go func() {
		if err := e.Start(address); err != nil {
			log.Infof("shutting down the server due error %v", err)
		}
	}()

	log.Info("Server started. Waiting for interrupt (2) (Ctrl+C)")
	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 10 seconds.
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Infof("Got signal %v - will forcibly close after %v", os.Interrupt, shutdownTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel() // releases resources if slowOperation completes before timeout elapses
	if err := e.Shutdown(ctx); err != nil {
		log.Fatal(err)
	} else {
		log.Infof("Server successfully shut down")
	}
}
