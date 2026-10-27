package media

import (
	"errors"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
)

var (
	inlineTypes = map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
		".gif": "image/gif", ".webp": "image/webp", ".avif": "image/avif",
		".bmp": "image/bmp", ".ico": "image/x-icon", ".heic": "image/heic",
		".mp4": "video/mp4", ".webm": "video/webm", ".ogv": "video/ogg",
		".mov": "video/quicktime", ".m4v": "video/x-m4v",
		".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "audio/ogg",
		".oga": "audio/ogg", ".m4a": "audio/mp4", ".flac": "audio/flac",
		".aac": "audio/aac", ".opus": "audio/opus",
	}

	keyPattern = regexp.MustCompile(`^/\d{4}/\d{2}/\d{2}/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\.[a-z0-9]{1,16}$`)
)

type notFoundNoStore struct {
	http.ResponseWriter
}

func (w notFoundNoStore) WriteHeader(code int) {
	if code >= http.StatusBadRequest {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.ResponseWriter.WriteHeader(code)
}

type controller struct {
	service *Service
	root    string
}

func newController(service *Service, root string) *controller {
	return &controller{service: service, root: root}
}

func (c *controller) registerProtected(r *gin.RouterGroup) {
	r.POST("/media", guard.RequirePresetGroup(rbac.GroupKeyOwner, rbac.GroupKeyAdmin), c.upload)
	r.DELETE("/media/:id", guard.RequirePresetGroup(rbac.GroupKeyOwner, rbac.GroupKeyAdmin), c.remove)
}

func (c *controller) upload(ctx *gin.Context) {
	if err := http.NewResponseController(ctx.Writer).SetReadDeadline(time.Time{}); err != nil {
		apierror.Write(ctx, apierror.ErrInternal)
		return
	}

	reader, err := ctx.Request.MultipartReader()
	if err != nil {
		apierror.Write(ctx, apierror.ErrInvalidRequest)
		return
	}

	identity, ok := guard.IdentityFrom(ctx)
	if !ok {
		apierror.Write(ctx, apierror.ErrInternal)
		return
	}
	uploaderID, err := uuid.Parse(identity.UserID)
	if err != nil {
		apierror.Write(ctx, apierror.ErrInternal)
		return
	}

	for {
		part, err := reader.NextPart()
		if err != nil {
			apierror.Write(ctx, apierror.ErrInvalidRequest)
			return
		}
		if part.FormName() != "file" {
			io.Copy(io.Discard, part)
			continue
		}

		asset, err := c.service.Upload(ctx.Request.Context(), uploaderID, part.FileName(), part)
		if err != nil {
			writeServiceError(ctx, err)
			return
		}
		url, err := c.service.URL(asset)
		if err != nil {
			apierror.Write(ctx, err)
			return
		}
		ctx.JSON(http.StatusCreated, mediaResponse{
			ID:           asset.ID.String(),
			URL:          url,
			SizeBytes:    asset.SizeBytes,
			OriginalName: asset.OriginalName,
		})
		return
	}
}

func (c *controller) remove(ctx *gin.Context) {
	id, err := uuid.Parse(ctx.Param("id"))
	if err != nil {
		apierror.Write(ctx, apierror.ErrNotFound)
		return
	}
	if err := c.service.Delete(ctx.Request.Context(), id); err != nil {
		writeServiceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, deleteResponse{ID: id.String(), Deleted: true})
}

func (c *controller) registerStorage(engine *gin.Engine) {
	fileServer := http.StripPrefix("/storage/", http.FileServer(http.Dir(c.root)))

	handler := func(ctx *gin.Context) {
		ctx.Header("X-Content-Type-Options", "nosniff")
		ctx.Header("Cache-Control", "public, max-age=31536000, immutable")

		if err := http.NewResponseController(ctx.Writer).SetWriteDeadline(time.Time{}); err != nil {
			ctx.Header("Cache-Control", "no-store")
			ctx.Status(http.StatusInternalServerError)
			return
		}

		key := ctx.Param("key")
		if !keyPattern.MatchString(key) {
			ctx.Header("Cache-Control", "no-store")
			ctx.Status(http.StatusNotFound)
			return
		}

		base := path.Base(key)
		if contentType, ok := inlineTypes[strings.ToLower(filepath.Ext(base))]; ok {
			ctx.Header("Content-Type", contentType)
		} else {
			ctx.Header("Content-Type", "application/octet-stream")
			ctx.Header("Content-Disposition", `attachment; filename="`+base+`"`)
		}
		fileServer.ServeHTTP(notFoundNoStore{ctx.Writer}, ctx.Request)
	}

	engine.GET("/storage/*key", handler)
	engine.HEAD("/storage/*key", handler)
}

func writeServiceError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, errNotFound):
		apierror.Write(ctx, apierror.ErrNotFound)
	case errors.Is(err, errInvalidOriginalName):
		apierror.Write(ctx, apierror.ErrInvalidRequest)
	case errors.Is(err, errUnknownProvider):
		apierror.Write(ctx, apierror.ErrInternal)
	case errors.Is(err, errStorage):
		apierror.Write(ctx, apierror.ErrInternal)
	default:
		apierror.Write(ctx, err)
	}
}
