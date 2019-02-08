package utils

import (
	"bytes"
	"golang.org/x/crypto/bcrypt"
	"io"
	"io/ioutil"
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

const USER_ID = "iserId"
const USER_LOGIN = "userLogin"
