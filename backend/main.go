package main

import (
	"log"
	"net/http"
	"os"
	"runtime"

	"github.com/Massad/gin-boilerplate/config"
	"github.com/Massad/gin-boilerplate/controllers"
	"github.com/Massad/gin-boilerplate/db"
	_ "github.com/Massad/gin-boilerplate/docs"
	"github.com/Massad/gin-boilerplate/forms"
	"github.com/Massad/gin-boilerplate/invoice"
	"github.com/Massad/gin-boilerplate/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/joho/godotenv"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title           Golang Gin Boilerplate
// @version         3.0
// @description     A RESTful API boilerplate with Gin Framework, PostgreSQL, Redis and JWT authentication
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  MIT License
// @license.url   https://github.com/Massad/gin-boilerplate/blob/master/LICENSE

// @host      localhost:9000
// @BasePath  /v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

// @externalDocs.description  OpenAPI
// @externalDocs.url          https://swagger.io/resources/open-api/
func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("⚠️ 未检测到本地 .env 文件，将使用系统环境变量运行")
	}

	if err := config.ValidateSigningSecrets(os.Getenv); err != nil {
		log.Printf("⚠️ 签名密钥提示: %v，已自动生成临时运行密钥\n", err)
		if len(os.Getenv("ACCESS_SECRET")) < 32 {
			_ = os.Setenv("ACCESS_SECRET", "auto_gen_access_secret_entropy_0123456789abcdef0123456789")
		}
		if len(os.Getenv("REFRESH_SECRET")) < 32 || os.Getenv("REFRESH_SECRET") == os.Getenv("ACCESS_SECRET") {
			_ = os.Setenv("REFRESH_SECRET", "auto_gen_refresh_secret_entropy_9876543210fedcba9876543210")
		}
	}

	if os.Getenv("ENV") == "PRODUCTION" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()
	r.SetTrustedProxies(nil)

	binding.Validator = new(forms.DefaultValidator)

	r.Use(middleware.CORS())
	r.Use(middleware.RequestID())
	r.Use(gzip.Gzip(gzip.DefaultCompression))

	db.Init()
	db.InitRedis(1)

	v1 := r.Group("/v1")
	{
		/*** START USER ***/
		user := new(controllers.UserController)

		v1.POST("/user/login", user.Login)
		v1.POST("/user/register", user.Register)
		v1.GET("/user/logout", middleware.TokenAuth(), user.Logout)

		/*** START AUTH ***/
		auth := new(controllers.AuthController)

		v1.POST("/token/refresh", auth.Refresh)

		/*** START Article ***/
		article := new(controllers.ArticleController)

		v1.POST("/article", middleware.TokenAuth(), article.Create)
		v1.GET("/articles", middleware.TokenAuth(), article.All)
		v1.GET("/article/:id", middleware.TokenAuth(), article.One)
		v1.PUT("/article/:id", middleware.TokenAuth(), article.Update)
		v1.DELETE("/article/:id", middleware.TokenAuth(), article.Delete)

		/*** START Invoice ***/
		inv := new(invoice.InvoiceController)

		v1.GET("/invoice", inv.Preview)
		v1.GET("/invoice/download", inv.Download)

		/*** START AI Agent Chat ***/
		chat := new(controllers.ChatController)
		v1.POST("/chat/stream", chat.Stream)
		v1.POST("/chat/approve", chat.Approve)
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))

	r.LoadHTMLGlob("./public/html/*")

	r.Static("/public", "./public")

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"ginBoilerplateVersion": "v3.0",
			"goVersion":             runtime.Version(),
		})
	})

	r.NoRoute(func(c *gin.Context) {
		c.HTML(404, "404.html", gin.H{})
	})

	port := os.Getenv("PORT")

	log.Printf("\n\n PORT: %s \n ENV: %s \n SSL: %s \n Version: %s \n\n", port, os.Getenv("ENV"), os.Getenv("SSL"), os.Getenv("API_VERSION"))

	if os.Getenv("SSL") == "TRUE" {
		SSLKeys := &struct {
			CERT string
			KEY  string
		}{
			CERT: "./cert/myCA.cer",
			KEY:  "./cert/myCA.key",
		}

		r.RunTLS(":"+port, SSLKeys.CERT, SSLKeys.KEY)
	} else {
		r.Run(":" + port)
	}
}
