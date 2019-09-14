package handlers

import (
	"context"
	"errors"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/gommon/log"
	"github.com/minio/minio-go"
	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/bson/primitive"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/options"
	"github.com/nkonev/blog-storage/utils"
	"github.com/spf13/viper"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"syscall"
)

type FsHandler struct {
	serverUrl string
	minio     *minio.Client
	mongo     *mongo.Client
}

type FileInfo struct {
	Id        string `json:"id"`
	Filename  string `json:"filename"`
	Url       string `json:"url"`
	PublicUrl string `json:"publicUrl"`
	Size      int64  `json:"size"`
}

type fileMongoDto struct {
	id        string // mongo document id equal to minio object jd
	filename  string
	published bool
}

type renameDto struct {
	Newname string `json:"newname"`
}

const id = "_id"
const filename = "filename"
const published = "published"
const FormFile = "file"
const userId = "userId"

func NewFsHandler(minio *minio.Client, serverUrl string, client *mongo.Client) *FsHandler {
	return &FsHandler{minio: minio, serverUrl: serverUrl, mongo: client}
}

func convertToFileMongoDto(elem bson.D) *fileMongoDto {
	filename := elem.Map()[filename].(string)
	id := elem.Map()[id].(primitive.ObjectID)
	published := elem.Map()[published].(bool)
	return &fileMongoDto{id.Hex(), filename, published}

}

func toFileMongoDto(c mongo.Cursor) (*fileMongoDto, error) {
	var elem bson.D
	err := c.Decode(&elem)
	if err != nil {
		return nil, err
	}

	return convertToFileMongoDto(elem), nil
}

func getIdDoc(objectId string) (bson.D, error) {
	ids, e := primitive.ObjectIDFromHex(objectId)
	if e != nil {
		return nil, e
	}
	ds := bson.D{{id, ids}}
	return ds, nil
}

func getUpdateDoc(p bson.M) bson.M {
	update := bson.M{"$set": p}
	return update
}

func (h *FsHandler) getMetainfoFromMongo(objectId string, userId int) (*fileMongoDto, error) {
	userFilesCollection := h.getUserCollectionInt(userId)
	ds, err := getIdDoc(objectId)
	if err != nil {
		log.Errorf("Error during creating id document %v", objectId)
		return nil, err
	}

	one := userFilesCollection.FindOne(context.TODO(), ds)
	if one == nil {
		return nil, errors.New("Unexpected nil by id " + objectId)
	}
	if one.Err() != nil {
		log.Errorf("Error during querying record from mongo by key %v", objectId)
		return nil, one.Err()
	}

	var elem bson.D
	if err := one.Decode(&elem); err != nil {
		if err == mongo.ErrNoDocuments {
			log.Errorf("No documents found by key %v", objectId)
		}
		return nil, err
	}
	return convertToFileMongoDto(elem), nil
}

const global_objects = "global_objects"

func (h *FsHandler) getNextGlobalId(userIdV int) (*string, error) {
	database := utils.GetMongoDatabase(h.mongo)
	ms := bson.M{userId: userIdV}
	result, e := database.Collection(global_objects).InsertOne(context.TODO(), ms)
	if e != nil {
		return nil, e
	}
	idMongo := result.InsertedID.(primitive.ObjectID).Hex()
	return &idMongo, nil
}

func (h *FsHandler) getUserIdByGlobalId(objectId string) (int, error) {
	ids, e := primitive.ObjectIDFromHex(objectId)
	if e != nil {
		return 0, e
	}
	database := utils.GetMongoDatabase(h.mongo)

	ms := bson.M{id: ids}
	one := database.Collection(global_objects).FindOne(context.TODO(), ms)
	if one.Err() != nil {
		if one.Err() != mongo.ErrNoDocuments {
			log.Errorf("Error during get user id by global id %v", objectId)
		} else {
			log.Infof("No documents found by global id %v", objectId)
		}
		return 0, one.Err()
	}
	var elem bson.D
	if err := one.Decode(&elem); err != nil {
		return 0, err
	}
	userId64 := elem.Map()[userId].(int64)
	return int(userId64), nil
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
	log.Debugf("Get userId: %v; userLogin: %v", c.Get(utils.USER_ID), c.Get(utils.USER_LOGIN))

	bucket := h.ensureAndGetBucket(c)

	log.Debugf("Listing bucket '%v':", bucket)

	userFilesCollection := h.getUserCollection(c)
	userFilesCursor, e := userFilesCollection.Find(context.TODO(), bson.D{})
	if e != nil {
		log.Errorf("Error during querying record from mongo")
		return e
	}
	defer userFilesCursor.Close(context.TODO())

	var list []FileInfo = make([]FileInfo, 0)
	for userFilesCursor.Next(context.TODO()) {
		mongoDto, err := toFileMongoDto(userFilesCursor)
		if err != nil {
			log.Errorf("Error during get mongo dto: %v", err)
			return err
		}

		obj, err := h.minio.GetObject(bucket, mongoDto.id, minio.GetObjectOptions{})
		if err != nil {
			log.Errorf("Error during GetObject: %v", err)
			return err
		}
		objInfo, err := obj.Stat()
		if err != nil {
			log.Errorf("Error during stat: %v", err)
			return err
		}
		log.Debugf("Object '%v'", objInfo.Key)

		publicUrl := ""
		if mongoDto.published {
			publicUrl = h.getPublicUrl(bucket, mongoDto.id)
		}

		downloadUrl, err := h.getPrivateUrlFromObject(objInfo)
		if err != nil {
			log.Errorf("Error get private url: %v", err)
			return err
		}

		info := FileInfo{Id: mongoDto.id, Filename: mongoDto.filename, Url: *downloadUrl, Size: objInfo.Size, PublicUrl: publicUrl}
		list = append(list, info)
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "files": list})
}

func (h *FsHandler) insertMetaInfoToMongo(c echo.Context, filename string, userId int) (*string, error) {

	globalId, err := h.getNextGlobalId(userId)
	if err != nil {
		log.Errorf("Error during create mongo global id document: %v", err)
		return nil, err
	}
	metainfoDocument, err := getMongoMetainfoDocument(globalId, filename)
	if err != nil {
		log.Errorf("Error during construct mongo global id document: %v", err)
		return nil, err
	}
	inserted, err := h.getUserCollection(c).InsertOne(context.TODO(), metainfoDocument)
	if err != nil {
		log.Errorf("Error during create mongo metadata document: %v", err)
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
		log.Errorf("Error during calculating max allowed %v", err)
		return false, err
	}
	if consumption+file.Size > maxAllowed {
		log.Infof("Upload too large %v+%v>%v bytes", consumption, file.Size, maxAllowed)
		return false, nil
	}
	return true, nil
}

func (h *FsHandler) UploadHandler(c echo.Context) error {

	file, err := c.FormFile(FormFile)
	if err != nil {
		log.Errorf("Error during extracting form %v parameter: %v", FormFile, err)
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

	log.Debugf("Determined content type: %v", contentType)

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
		log.Errorf("Error during upload object: %v", err)
		return err
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "id": mongoId})
}

func (h *FsHandler) getUserCollection(c echo.Context) *mongo.Collection {
	database := utils.GetMongoDatabase(h.mongo)
	bucketName := getBucketName(c)
	return database.Collection(bucketName)
}

func (h *FsHandler) getUserCollectionInt(userId int) *mongo.Collection {
	database := utils.GetMongoDatabase(h.mongo)
	bucketName := getBucketNameInt(userId)
	return database.Collection(bucketName)
}

func getMongoMetainfoDocument(globalId *string, file string) (bson.D, error) {
	ids, e := primitive.ObjectIDFromHex(*globalId)
	if e != nil {
		return nil, e
	}
	return bson.D{{id, ids}, {filename, file}, {published, false}}, nil
}

func getBucketName(c echo.Context) string {
	i, _ := getUserIdFromRequest(c)
	return getBucketNameInt(i)
}

func getUserIdFromRequest(c echo.Context) (int, error) {
	i, ok := c.Get(utils.USER_ID).(int)
	if !ok {
		log.Errorf("Error during get(cast) userId")
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
			log.Debugf("Bucket '%s' already present", bucketName)
		} else {
			log.Fatal(err)
		}
	} else {
		log.Infof("Successfully created bucket '%s'", bucketName)
	}

}

func (h *FsHandler) download(bucketName, objId string, userId int) func(c echo.Context) error {
	return func(c echo.Context) error {
		info, e := h.minio.StatObject(bucketName, objId, minio.StatObjectOptions{})
		if e != nil {
			return c.JSON(http.StatusNotFound, &utils.H{"status": "stat fail"})
		}

		c.Response().Header().Set(echo.HeaderContentLength, strconv.FormatInt(info.Size, 10))
		c.Response().Header().Set(echo.HeaderContentType, info.ContentType)
		mongoDto, err := h.getMetainfoFromMongo(objId, userId)
		if err != nil {
			return err
		}
		c.Response().Header().Set(echo.HeaderContentDisposition, "attachment; filename=\""+mongoDto.filename+"\"")

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

	objName := getFileId(c)
	i, e := getUserIdFromRequest(c)
	if e != nil {
		return e
	}

	return h.download(bucketName, objName, i)(c)
}

func (h *FsHandler) PublicDownloadHandler(c echo.Context) error {

	objId := getFileId(c)

	userId, err := h.getUserIdByGlobalId(objId)

	dto, err := h.getMetainfoFromMongo(objId, userId)

	if err != nil {
		return err
	}

	if err == mongo.ErrNoDocuments || !dto.published {
		return c.JSON(http.StatusNotFound, &utils.H{"status": "access fail"})
	}

	bucketName := getBucketNameInt(userId)

	return h.download(bucketName, objId, userId)(c)
}

func getFileId(context echo.Context) string {
	return context.Param("file")
}

func (h *FsHandler) MoveHandler(c echo.Context) error {
	from := getFileId(c)
	u := &renameDto{}
	if err := c.Bind(u); err != nil {
		return err
	}

	userFilesCollection := h.getUserCollection(c)
	findDocument, err := getIdDoc(from)
	if err != nil {
		return err
	}
	updateDocument := getUpdateDoc(primitive.M{filename: u.Newname})

	one := userFilesCollection.FindOneAndUpdate(context.TODO(), findDocument, updateDocument)
	if one == nil {
		return errors.New("Unexpected nil result during update")
	}
	if one.Err() != nil {
		return one.Err()
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func (h *FsHandler) DeleteHandler(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)
	objId := getFileId(c)

	if err := h.minio.RemoveObject(bucketName, objId); err != nil {
		log.Errorf("Error during remove object from minio: %v", err)
		return c.JSON(http.StatusInternalServerError, &utils.H{"status": "fail"})
	}

	userFilesCollection := h.getUserCollection(c)
	findDocument, err := getIdDoc(objId)
	if err != nil {
		return err
	}
	_, e := userFilesCollection.DeleteOne(context.TODO(), findDocument)
	if e != nil {
		log.Errorf("Error during remove object from mongo: %v", e)
		return e
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok"})
}

func (h *FsHandler) Limits(c echo.Context) error {
	bucketName := h.ensureAndGetBucket(c)

	userId, ok := c.Get(utils.USER_ID).(int)
	if !ok {
		log.Errorf("Error during get(cast) userId")
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

	log.Debugf("Listing bucket '%v':", bucketName)
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

	collection := h.getUserCollection(c)
	findDocument, err := getIdDoc(objId)
	if err != nil {
		return err
	}

	updateDocument := getUpdateDoc(primitive.M{published: true})

	one := collection.FindOneAndUpdate(context.TODO(), findDocument, updateDocument)
	if one == nil {
		return errors.New("Unexpected nil result during update")
	}
	if one.Err() != nil {
		return one.Err()
	}
	var elem bson.D
	if err := one.Decode(&elem); err != nil {
		return err
	}
	dto := convertToFileMongoDto(elem)

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "published": true, "url": h.getPublicUrl(getBucketName(c), dto.id)})
}

func (h *FsHandler) isDocumentExists(collection string, request interface{}, opts ...*options.FindOneOptions) (bool, error) {
	database := utils.GetMongoDatabase(h.mongo)

	// https://siongui.github.io/2017/03/13/go-pass-slice-or-array-as-variadic-parameter/#id12
	res := database.Collection(collection).FindOne(context.TODO(), request, opts[:]...)
	if res.Err() != nil {
		log.Errorf("Error during find '%v' : %v", request, res.Err())
		return false, res.Err()
	}

	_, e := res.DecodeBytes()

	if e != nil {
		if e == mongo.ErrNoDocuments {
			return false, nil
		} else {
			log.Errorf("Error during DecodeBytes '%v' : %v", request, res.Err())
			return false, e
		}
	} else {
		return true, nil
	}

}

func (h *FsHandler) DeletePublish(c echo.Context) error {
	objId := getFileId(c)

	collection := h.getUserCollection(c)
	findDocument, err := getIdDoc(objId)
	if err != nil {
		return err
	}
	updateDocument := getUpdateDoc(primitive.M{published: false})

	one := collection.FindOneAndUpdate(context.TODO(), findDocument, updateDocument)
	if one == nil {
		return errors.New("Unexpected nil result during update")
	}
	if one.Err() != nil {
		return one.Err()
	}

	return c.JSON(http.StatusOK, &utils.H{"status": "ok", "unpublished": true})
}

func (h *FsHandler) getMaxAllowedConsumption(userId int) (int64, error) {
	b, e := h.isDocumentExists("limits", bson.D{{id, userId}})
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
