package main

import (
	"context"
	"encoding/json"
	"github.com/GeertJohan/go.rice"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mongodb"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/minio/minio-go"
	"github.com/nkonev/blog-storage/client"
	"github.com/nkonev/blog-storage/data/migrate_rice"
	"github.com/nkonev/blog-storage/data/mongo_lock"
	"github.com/nkonev/blog-storage/data/repository"
	"github.com/nkonev/blog-storage/handlers"
	. "github.com/nkonev/blog-storage/logger"
	"github.com/nkonev/blog-storage/utils"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/fx"
	"net/http"
	"regexp"
	"strings"
)

const SESSION_COOKIE = "SESSION"
const AUTH_URL = "auth.url"
const LOCK_COLLECTION = "migration_lock"

type authMiddleware echo.MiddlewareFunc
type staticMiddleware echo.MiddlewareFunc

func main() {
	utils.InitViper("./config-dev/config.yml")

	app := fx.New(
		fx.Logger(Logger),
		fx.Provide(
			configureMongo,
			configureMinio,
			repository.NewUserFileRepository,
			repository.NewLimitsRepository,
			handlers.NewFsHandler,
			configureEcho,
			configureMigrate,
			configureAuthMiddleware,
			configureStaticMiddleware,
			client.NewRestClient,
		),
		fx.Invoke(runMigrate, runEcho),
	)
	app.Run()

	Logger.Infof("Exit program")
}

func configureEcho(fsh *handlers.FsHandler, authMiddleware authMiddleware, staticMiddleware staticMiddleware, lc fx.Lifecycle) *echo.Echo {
	bodyLimit := viper.GetString("server.body.limit")

	e := echo.New()
	e.Logger.SetOutput(Logger.Writer())

	e.Pre(echo.MiddlewareFunc(staticMiddleware))
	e.Use(echo.MiddlewareFunc(authMiddleware))

	accessLoggerConfig := middleware.LoggerConfig{
		Output: Logger.Writer(),
		Format: `"remote_ip":"${remote_ip}",` +
			`"method":"${method}","uri":"${uri}",` +
			`"status":${status},"error":"${error}","latency_human":"${latency_human}"` +
			`,"bytes_in":${bytes_in},"bytes_out":${bytes_out},"user_agent":"${user_agent}"` + "\n",
	}
	e.Use(middleware.LoggerWithConfig(accessLoggerConfig))
	e.Use(middleware.Secure())
	e.Use(middleware.BodyLimit(bodyLimit))

	e.GET("/ls", fsh.LsHandler)
	e.GET("/limits", fsh.Limits)
	e.POST("/upload", fsh.UploadHandler)
	e.GET(utils.DOWNLOAD_PREFIX+":file", fsh.DownloadHandler)
	e.POST("/rename/:file", fsh.MoveHandler)
	e.DELETE("/delete/:file", fsh.DeleteHandler)
	e.PUT("/publish/:file", fsh.Publish)
	e.GET(utils.PUBLIC_PREFIX+"/"+utils.USER_PREFIX+":userId/:file", fsh.PublicDownloadHandler)
	e.DELETE("/publish/:file", fsh.DeletePublish)

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			// do some work on application stop (like closing connections and files)
			Logger.Infof("Stopping server")
			return e.Shutdown(ctx)
		},
	})

	return e
}

func configureStaticMiddleware() staticMiddleware {
	box := rice.MustFindBox("static").HTTPBox()

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
			Logger.Infof("Skipping authentication for %v because it matches %v", uri, regexp0.String())
			return true
		}
	}
	return false
}

func configureAuthMiddleware(httpClient client.RestClient) authMiddleware {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			whitelistStr := viper.GetStringSlice("auth.exclude")
			whitelist := utils.StringsToRegexpArray(whitelistStr)
			if checkUrlInWhitelist(whitelist, c.Request().RequestURI) {
				return next(c)
			}

			sessionCookie, err := c.Request().Cookie(SESSION_COOKIE)
			if err != nil {
				Logger.Infof("Error get '%v' cookie: %v", SESSION_COOKIE, err)
				return c.JSON(http.StatusUnauthorized, &utils.H{"status": "unauthorized"})
			}

			authUrl := viper.GetString(AUTH_URL)
			// check cookie
			req, err := http.NewRequest(
				"GET", authUrl, nil,
			)
			if err != nil {
				Logger.Errorf("Error during create request: %v", err)
				return err
			}

			req.AddCookie(sessionCookie)
			req.Header.Add("Accept", "application/json")
			resp, err := httpClient.Do(req)
			if err != nil {
				Logger.Errorf("Error during requesting auth backend: %v", err)
				return err
			}
			defer resp.Body.Close()

			// put user id, user name to context
			b := resp.Body
			decoder := json.NewDecoder(b)
			var decodedResponse interface{}
			err = decoder.Decode(&decodedResponse)
			if err != nil {
				Logger.Errorf("Error during decoding json: %v", err)
				return err
			}

			if resp.StatusCode == 401 {
				return c.JSON(resp.StatusCode, &utils.H{"status": "unauthorized"})
			} else if resp.StatusCode == 200 {
				dto := decodedResponse.(map[string]interface{})
				i, ok := dto["id"].(float64)
				if !ok {
					Logger.Errorf("Error during casting to int")
					return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
				}
				c.Set(utils.USER_ID, int(i))
				c.Set(utils.USER_LOGIN, dto["login"])
				return next(c)
			} else {
				Logger.Errorf("Unknown auth status %v", resp.StatusCode)
				return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
			}

		}
	}
}

func configureMigrate() *migrate.Migrate {
	box := rice.MustFindBox("data/migrations")

	driver, err := migrate_rice.WithInstance(box)
	if err != nil {
		Logger.Panicf("Error during create migrator driver: %v", err)
	}

	m, err := migrate.NewWithSourceInstance(migrate_rice.Name, driver, utils.GetMongoUrl())

	if err != nil {
		Logger.Panicf("Error during create migrator: %v", err)
	}
	return m
}

func runMigrate(m *migrate.Migrate) {
	mongoClient := utils.GetMongoClient()
	defer mongoClient.Disconnect(context.TODO())

	lock := mongo_lock.NewMongoLock(mongoClient, LOCK_COLLECTION)
	lock.AcquireLock()

	err := m.Up()
	defer m.Close()
	if err != nil {
		if err.Error() == "no change" {
			Logger.Info("Migration(s) already applied")
		} else {
			Logger.Panicf("Error during applying migrations: %v", err)
		}
	}
	Logger.Info("Migration run successfully")

	lock.ReleaseLock()
}

func configureMinio() *minio.Client {
	endpoint := viper.GetString("minio.endpoint")
	accessKeyID := viper.GetString("minio.accessKeyId")
	secretAccessKey := viper.GetString("minio.secretAccessKey")
	useSSL := false

	// Initialize minio client object.
	minioClient, err := minio.New(endpoint, accessKeyID, secretAccessKey, useSSL)
	if err != nil {
		Logger.Fatal(err)
	}

	return minioClient
}

func configureMongo(lc fx.Lifecycle) *mongo.Client {
	mongoClient := utils.GetMongoClient()
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			// do some work on application stop (like closing connections and files)
			Logger.Infof("Stopping mongo client")
			return mongoClient.Disconnect(ctx)
		},
	})

	return mongoClient
}

// rely on viper import and it's configured by
func runEcho(e *echo.Echo) {
	address := viper.GetString("server.address")

	Logger.Info("Starting server...")
	// Start server in another goroutine
	go func() {
		if err := e.Start(address); err != nil {
			Logger.Infof("server shut down: %v", err)
		}
	}()
	Logger.Info("Server started. Waiting for interrupt (2) (Ctrl+C)")
}
