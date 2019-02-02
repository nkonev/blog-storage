package main

import (
	"context"
	"fmt"
	"github.com/gobuffalo/packr"
	"github.com/nkonev/blog-store/handlers"
	"github.com/spf13/viper"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/labstack/gommon/log"
	"go.uber.org/dig"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mongodb"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/minio/minio-go"
)

func configureEcho() *echo.Echo {
	bodyLimit := viper.GetString("server.body.limit")

	log.SetOutput(os.Stdout)

	static := packr.NewBox("./static")

	e := echo.New()

	e.Use(getAuthMiddleware(stringsToRegexpArray("/user.*", "/auth/.*", "/confirm.*")))

	e.Use(middleware.Logger())
	e.Use(middleware.Secure())
	e.Use(middleware.BodyLimit(bodyLimit))

	e.GET("/ls", handlers.LsHandler)
	e.POST("/upload", handlers.UploadHandler)
	e.GET("/download", handlers.DownloadHandler)
	e.POST("/move", handlers.MoveHandler)
	e.DELETE("/delete", handlers.DeleteHandler)

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

func stringsToRegexpArray(strings ...string) []regexp.Regexp {
	regexps := make([]regexp.Regexp, len(strings))
	for i, str := range strings {
		r, err := regexp.Compile(str)
		if err != nil {
			panic(err)
		} else {
			regexps[i] = *r
		}
	}
	return regexps
}

func getAuthMiddleware(whitelist []regexp.Regexp) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// TODO check auth - extract session auth cookie value and check it with blog
			return next(c)
		}
	}
}

func main() {
	initViper()
	container := dig.New()
	container.Provide(configureEcho)
	container.Provide(configureMigrate)

	container.Invoke(runMigrate)

	runMinio()

	if echoErr := container.Invoke(runEcho); echoErr != nil {
		log.Fatalf("Error during invoke echo: %v", echoErr)
	}
	log.Infof("Exit program")
}

func configureMigrate() *migrate.Migrate {
	m, err := migrate.New(
		viper.GetString("mongo.migrations.sourceUrl"),
		viper.GetString("mongo.migrations.databaseUrl"),
	)
	if err != nil {
		log.Panicf("Error during create migrator", err)
	}
	return m
}

func runMigrate(m *migrate.Migrate) {
	err := m.Up()
	if err != nil {
		if err.Error() == "no change" {
			log.Info("Migration(s) already applied")
		} else {
			log.Panicf("Error during applying migrations", err)
		}
	}
	log.Info("Migration run successfully")
}

func runMinio() {
	endpoint := viper.GetString("minio.endpoint")
	accessKeyID := viper.GetString("minio.accessKeyId")
	secretAccessKey := viper.GetString("minio.secretAccessKey")
	useSSL := false

	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, accessKeyID, secretAccessKey, useSSL)
	if err != nil {
		log.Fatal(err)
	}

	// Make a new bucket called mymusic.
	bucketName := "mymusic"
	location := "us-east-1"

	err = minioClient.MakeBucket(bucketName, location)
	if err != nil {
		// Check to see if we already own this bucket (which happens if you run this twice)
		exists, err := minioClient.BucketExists(bucketName)
		if err == nil && exists {
			log.Printf("We already own %s", bucketName)
		} else {
			log.Fatal(err)
		}
	} else {
		log.Printf("Successfully created %s", bucketName)
	}

	// Upload the zip file
	objectName := "001_create_user.up.json"
	filePath := "./migrations/001_create_user.up.json"
	contentType := "application/json"

	// Upload the zip file with FPutObject
	n, err := minioClient.FPutObject(bucketName, objectName, filePath, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Successfully uploaded %s of size %d", objectName, n)
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
