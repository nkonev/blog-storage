package handlers

import (
	"github.com/labstack/echo"
	"github.com/nkonev/blog-store/utils"
	"net/http"
)

func LsHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func UploadHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func DownloadHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func MoveHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func DeleteHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}