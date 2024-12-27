package _app

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"net/http"
)

type BaseAPI struct {
	Engine *gin.Engine
}

func (b BaseAPI) RegisterSwaggerRoutes() {
	b.Engine.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/swagger/index.html")
	})
	b.Engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

func (b BaseAPI) Success(c *gin.Context, data interface{}) {
	if data != nil {
		c.JSON(http.StatusOK, data)
	}
}

func (b BaseAPI) Error(c *gin.Context, err error) {
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
	}
}
