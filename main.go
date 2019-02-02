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
			// TODO check auth
			return next(c)
		}
	}
}


func main() {
	initViper()
	container := dig.New()
	container.Provide(configureEcho)


	if echoErr := container.Invoke(runEcho); echoErr != nil {
		log.Fatalf("Error during invoke echo: %v", echoErr)
	}
	log.Infof("Exit program")
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