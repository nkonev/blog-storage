package utils

import (
	"github.com/labstack/gommon/log"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/x/network/connstring"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
	"regexp"
)

type H map[string]interface{}

func HashPassword(password string) (string, error) {
	passwordHash, passwordHashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if passwordHashErr != nil {
		return "", passwordHashErr
	}
	return string(passwordHash), nil
}

func StringsToRegexpArray(strings []string) []regexp.Regexp {
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

func GetMongoDbName(mongoUrl string) string {
	uri, err := connstring.Parse(mongoUrl)
	if err != nil {
		log.Panicf("Error during parsing url: %v", err)
	}

	return uri.Database
}

func GetMongoUrl() string {
	return viper.GetString("mongo.migrations.databaseUrl")
}

func GetMongoDatabase(client *mongo.Client) *mongo.Database {
	return client.Database(GetMongoDbName(GetMongoUrl()))
}

const USER_ID = "iserId"
const USER_LOGIN = "userLogin"
const DOWNLOAD_PREFIX = "/download/"
