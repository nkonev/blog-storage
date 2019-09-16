package handlers

import (
	"context"
	"errors"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/minio/minio-go"
	"github.com/nkonev/blog-storage/data/repository"
	. "github.com/nkonev/blog-storage/logger"
	"github.com/nkonev/blog-storage/utils"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"syscall"
)

type FsHandler struct {
	serverUrl          string
	minio              *minio.Client
	mongo              *mongo.Client
	userFileRepository *repository.UserFileRepository
	glogalIdRepository *repository.GlogalIdRepository
}

type RenameDto struct {
	Newname string `json:"newname"`
}

type FileInfoDto struct {
	Id        string `json:"id"`
	Filename  string `json:"filename"`
	Url       string `json:"url"`
	PublicUrl string `json:"publicUrl"`
	Size      int64  `json:"size"`
}

const FormFile = "file"
const collectionLimits = "limits"

func NewFsHandler(minio *minio.Client, client *mongo.Client,
	userFileRepository *repository.UserFileRepository, glogalIdRepository *repository.GlogalIdRepository) *FsHandler {
	return &FsHandler{minio: minio, serverUrl: viper.GetString("server.url"), mongo: client, userFileRepository: userFileRepository, glogalIdRepository: glogalIdRepository}
}

func (h *FsHandler) getPrivateUrlFromObject(objInfo minio.ObjectInfo) (*string, error) {
	downloadUrl, err := url.Parse(h.serverUrl)
	if err != nil {
		return nil, err
	}
	downloadUrl.Path += utils.DOWNLOAD_PREFIX + objInfo.Key
	str := downloadUrl.String()
	return &str, nil
}

func (h *FsHandler) LsHandler(c echo.Context) error {
	Logger.Debugf("Get userId: %v; userLogin: %v", c.Get(utils.USER_ID), c.Get(utils.USER_LOGIN))

	bucket := h.ensureAndGetBucket(c)

	Logger.Debugf("Listing bucket '%v':", bucket)

	userFilesCollection := h.getUserCollection(c)
	userFilesCursor, e := userFilesCollection.Find(context.TODO(), bson.D{})
	if e != nil {
		Logger.Errorf("Error during querying record from mongo")
		return e
	}
	defer userFilesCursor.Close(context.TODO())

	var list []FileInfoDto = make([]FileInfoDto, 0)
	for userFilesCursor.Next(context.TODO()) {
		mongoDto, err := repository.ToFileMongoDto(userFilesCursor)
		if err != nil {
			Logger.Errorf("Error during get mongo dto: %v", err)
			return err
		}

		obj, err := h.minio.GetObject(bucket, mongoDto.Id.Hex(), minio.GetObjectOptions{})
		if err != nil {
			Logger.Errorf("Error during GetObject: %v", err)
			return err
		}
		objInfo, err := obj.Stat()
		if err != nil {
			Logger.Errorf("Error during stat: %v", err)
			return err
		}
		Logger.Debugf("Object '%v'", objInfo.Key)

		publicUrl := ""
		if mongoDto.Published {
			publicUrl = h.getPublicUrl(bucket, mongoDto.Id.Hex())
		}

		downloadUrl, err := h.getPrivateUrlFromObject(objInfo)
		if err != nil {
			Logger.Errorf("Error get private url: %v", err)
			return err
		}

		info := FileInfoDto{Id: mongoDto.Id.Hex(), Filename: mongoDto.Filename, Url: *downloadUrl, Size: objInfo.Size, PublicUrl: publicUrl}
		list = append(list, info)
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "files": list})
}

func (h *FsHandler) insertMetaInfoToMongo(c echo.Context, filename string, userId int) (*string, error) {

	globalId, err := h.glogalIdRepository.GetNextGlobalId(userId)
	if err != nil {
		Logger.Errorf("Error during create mongo global id document: %v", err)
		return nil, err
	}
	ids, err := primitive.ObjectIDFromHex(*globalId)
	if err != nil {
		Logger.Errorf("Error during convert id: %v", err)
		return nil, err
	}

	inserted, err := h.getUserCollection(c).InsertOne(context.TODO(), repository.UserFileDto{Id: ids, Filename: filename, Published: false})
	if err != nil {
		Logger.Errorf("Error during create mongo metadata document: %v", err)
		return nil, err
	}
	idMongo := inserted.InsertedID.(primitive.ObjectID).Hex()
	return &idMongo, nil
}

func (h *FsHandler) checkUserLimit(bucketName string, c echo.Context, file *multipart.FileHeader) (bool, error) {
	consumption := h.calcUserFilesConsumption(bucketName)
	userId, ok := c.Get(utils.USER_ID).(int)
	if !ok {
		return false, errors.New("Error during get(cast) userId")
	}
	maxAllowed, err := h.getMaxAllowedConsumption(userId)
	if err != nil {
		Logger.Errorf("Error during calculating max allowed %v", err)
		return false, err
	}
	if consumption+file.Size > maxAllowed {
		Logger.Infof("Upload too large %v+%v>%v bytes", consumption, file.Size, maxAllowed)
		return false, nil
	}
	return true, nil
}

func (h *FsHandler) UploadHandler(c echo.Context) error {

	file, err := c.FormFile(FormFile)
	if err != nil {
		Logger.Errorf("Error during extracting form %v parameter: %v", FormFile, err)
		return err
	}

	bucketName := h.ensureAndGetBucket(c)

	userLimitOk, err := h.checkUserLimit(bucketName, c, file)
	if err != nil {
		return err
	}
	if !userLimitOk {
		return c.JSON(http.StatusRequestEntityTooLarge, &utils.H{"status": "fail"})
	}

	contentType := file.Header.Get("Content-Type")

	Logger.Debugf("Determined content type: %v", contentType)

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	// put file
	i, err := getUserIdFromRequest(c)
	if err != nil {
		return err
	}
	mongoId, err := h.insertMetaInfoToMongo(c, file.Filename, i)
	if err != nil {
		return err
	}

	if _, err := h.minio.PutObject(bucketName, *mongoId, src, file.Size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		Logger.Errorf("Error during upload object: %v", err)
		return err
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "id": mongoId})
}

func (h *FsHandler) getUserCollection(c echo.Context) *mongo.Collection {
	database := utils.GetMongoDatabase(h.mongo)
	bucketName := getBucketName(c)
	return database.Collection(bucketName)
}

func (h *FsHandler) GetUserCollectionInt(userId int) *mongo.Collection {
	database := utils.GetMongoDatabase(h.mongo)
	bucketName := getBucketNameInt(userId)
	return database.Collection(bucketName)
}

func getBucketName(c echo.Context) string {
	i, _ := getUserIdFromRequest(c)
	return getBucketNameInt(i)
}

func getUserIdFromRequest(c echo.Context) (int, error) {
	i, ok := c.Get(utils.USER_ID).(int)
	if !ok {
		Logger.Errorf("Error during get(cast) userId")
		return 0, errors.New("Error during get(cast) userId")
	}
	return i, nil
}

func getBucketNameInt(userId interface{}) string {
	return fmt.Sprintf(utils.USER_PREFIX+"%v", userId)
}

func getBucketLocation(c echo.Context) string {
	return "europe-east"
}

func (h *FsHandler) ensureAndGetBucket(c echo.Context) string {
	bucketName := getBucketName(c)
	bucketLocation := getBucketLocation(c)
	h.ensureBucket(bucketName, bucketLocation)
	return bucketName
}

func (h *FsHandler) ensureBucket(bucketName, location string) {
	err := h.minio.MakeBucket(bucketName, location)
	if err != nil {
		// Check to see if we already own this bucket (which happens if you run this twice)
		exists, err := h.minio.BucketExists(bucketName)
		if err == nil && exists {
			Logger.Debugf("Bucket '%s' already present", bucketName)
		} else {
			Logger.Fatal(err)
		}
	} else {
		Logger.Infof("Successfully created bucket '%s'", bucketName)
	}

}

func (h *FsHandler) download(bucketName, objId string, mongoDto *repository.UserFileDto) func(c echo.Context) error {
	return func(c echo.Context) error {
		info, e := h.minio.StatObject(bucketName, objId, minio.StatObjectOptions{})
		if e != nil {
			return c.JSON(http.StatusNotFound, &utils.H{"status": "stat fail"})
		}

		c.Response().Header().Set(echo.HeaderContentLength, strconv.FormatInt(info.Size, 10))
		c.Response().Header().Set(echo.HeaderContentType, info.ContentType)
		c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; Filename=\""+mongoDto.Filename+"\"")

		object, e := h.minio.GetObject(bucketName, objId, minio.GetObjectOptions{})
		defer object.Close()
		if e != nil {
			return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
		}

		return c.Stream(http.StatusOK, info.ContentType, object)
	}
}

func (h *FsHandler) DownloadHandler(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)

	objId := getFileId(c)
	userId, e := getUserIdFromRequest(c)
	if e != nil {
		return e
	}

	dto, err := h.userFileRepository.GetMetainfoFromMongo(objId, getBucketNameInt(userId))
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, &utils.H{"status": "stat fail"})
		}
		return err
	}

	return h.download(bucketName, objId, dto)(c)
}

func (h *FsHandler) PublicDownloadHandler(c echo.Context) error {

	objId := getFileId(c)

	userId, err := h.glogalIdRepository.GetUserIdByGlobalId(objId)

	dto, err := h.userFileRepository.GetMetainfoFromMongo(objId, getBucketNameInt(userId))
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return c.JSON(http.StatusNotFound, &utils.H{"status": "stat fail"})
		}
		return err
	}
	if !dto.Published {
		return c.JSON(http.StatusNotFound, &utils.H{"status": "access fail"})
	}

	bucketName := getBucketNameInt(userId)

	return h.download(bucketName, objId, dto)(c)
}

func getFileId(context echo.Context) string {
	return context.Param("file")
}

func (h *FsHandler) MoveHandler(c echo.Context) error {
	from := getFileId(c)

	u := &RenameDto{}
	if err := c.Bind(u); err != nil {
		return err
	}

	bucketName := getBucketName(c)

	if err := h.userFileRepository.RenameUserFile(from, u.Newname, bucketName); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func (h *FsHandler) DeleteHandler(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)
	objId := getFileId(c)

	if err := h.minio.RemoveObject(bucketName, objId); err != nil {
		Logger.Errorf("Error during remove object from minio: %v", err)
		return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
	}

	userFilesCollection := h.getUserCollection(c)
	findDocument, err := repository.GetIdDoc(objId)
	if err != nil {
		return err
	}
	_, e := userFilesCollection.DeleteOne(context.TODO(), findDocument)
	if e != nil {
		Logger.Errorf("Error during remove object from mongo: %v", e)
		return e
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func (h *FsHandler) Limits(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)

	userId, ok := c.Get(utils.USER_ID).(int)
	if !ok {
		Logger.Errorf("Error during get(cast) userId")
	}

	max, e := h.getMaxAllowedConsumption(userId)
	if e != nil {
		return e
	}
	consumption := h.calcUserFilesConsumption(bucketName)

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "used": h.calcUserFilesConsumption(bucketName), "available": max - consumption})
}

func (h *FsHandler) calcUserFilesConsumption(bucketName string) int64 {
	var totalBucketConsumption int64

	recursive := true
	doneCh := make(chan struct{})
	defer close(doneCh)

	Logger.Debugf("Listing bucket '%v':", bucketName)
	for objInfo := range h.minio.ListObjects(bucketName, "", recursive, doneCh) {
		totalBucketConsumption += objInfo.Size
	}
	return totalBucketConsumption
}

func (h *FsHandler) getPublicUrl(bucketName string, minioObjId string) string {
	return h.serverUrl + utils.PUBLIC_PREFIX + "/" + bucketName + "/" + minioObjId
}

func (h *FsHandler) Publish(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)

	objId := getFileId(c)

	_, e := h.minio.StatObject(bucketName, objId, minio.StatObjectOptions{})
	if e != nil {
		return c.JSON(http.StatusNotFound, &utils.H{"status": "stat fail"})
	}

	elem, err := h.userFileRepository.UpdatePublished(getBucketName(c), objId, true)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "Published": true, "url": h.getPublicUrl(getBucketName(c), elem.Id.Hex())})
}

func (h *FsHandler) DeletePublish(c echo.Context) error {
	objId := getFileId(c)

	_, err := h.userFileRepository.UpdatePublished(getBucketName(c), objId, false)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "unpublished": true})
}

func (h *FsHandler) getMaxAllowedConsumption(userId int) (int64, error) {
	b, e := repository.IsDocumentExists(h.mongo, collectionLimits, bson.D{{repository.Id, userId}})
	if e != nil {
		return 0, e
	}

	if b {
		var stat syscall.Statfs_t
		wd := viper.GetString("limits.stat.dir")
		err := syscall.Statfs(wd, &stat)
		if err != nil {
			return 0, err
		}
		// Available blocks * size per block = available space in bytes
		u := int64(stat.Bavail * uint64(stat.Bsize))
		return u, nil
	} else {
		return viper.GetInt64("limits.default.per.user.max"), nil
	}
}
