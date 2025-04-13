package app

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/director74/dz7_shop/notification-service/config"
	httpController "github.com/director74/dz7_shop/notification-service/internal/controller/http"
	"github.com/director74/dz7_shop/notification-service/internal/entity"
	"github.com/director74/dz7_shop/notification-service/internal/repo"
	"github.com/director74/dz7_shop/notification-service/internal/usecase"
	"github.com/director74/dz7_shop/pkg/database"
	"github.com/director74/dz7_shop/pkg/errors"
	"github.com/director74/dz7_shop/pkg/messaging"
	"github.com/director74/dz7_shop/pkg/rabbitmq"
)

// App представляет приложение
type App struct {
	config     *config.Config
	httpServer *http.Server
	db         *gorm.DB
	router     *gin.Engine
	rabbitMQ   *rabbitmq.RabbitMQ
}

func NewApp(config *config.Config) (*App, error) {
	var db *gorm.DB
	var rmq *rabbitmq.RabbitMQ
	var err error

	// Инициализируем подключение к PostgreSQL
	db, err = database.NewPostgresDB(config.Postgres)
	if err != nil {
		return nil, errors.AppendPrefix(err, "не удалось подключиться к базе данных")
	}

	// Автомиграция моделей
	if err := database.AutoMigrateWithCleanup(db, &entity.Notification{}); err != nil {
		return nil, errors.AppendPrefix(err, "не удалось выполнить миграцию")
	}

	// Инициализируем подключение к RabbitMQ
	rmq, err = messaging.InitRabbitMQ(config.RabbitMQ)
	if err != nil {
		database.CloseDB(db)
		return nil, errors.AppendPrefix(err, "не удалось подключиться к RabbitMQ")
	}

	// Инициализируем Gin роутер
	router := gin.Default()

	// Добавляем middleware для обработки ошибок и восстановления после паники
	router.Use(errors.RecoveryMiddleware())
	router.Use(errors.ErrorMiddleware())

	// Настраиваем обработчики для 404 и 405 ошибок
	router.NoRoute(errors.NotFoundHandler())
	router.NoMethod(errors.MethodNotAllowedHandler())

	httpServer := &http.Server{
		Addr:         ":" + config.HTTP.Port,
		Handler:      router,
		ReadTimeout:  config.HTTP.ReadTimeout,
		WriteTimeout: config.HTTP.WriteTimeout,
	}

	return &App{
		config:     config,
		httpServer: httpServer,
		db:         db,
		router:     router,
		rabbitMQ:   rmq,
	}, nil
}

// Run запускает приложение
func (a *App) Run() error {
	// Настраиваем обработку сигналов завершения
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Инициализируем зависимости
	notificationRepo := repo.NewNotificationRepository(a.db)

	// Для тестирования используем DummyEmailSender
	emailSender := usecase.NewDummyEmailSender()
	// Для реального использования SMTP:
	// emailSender := usecase.NewSmtpEmailSender(
	//     a.config.Mail.SMTPHost,
	//     a.config.Mail.SMTPPort,
	//     a.config.Mail.SMTPUser,
	//     a.config.Mail.SMTPPassword,
	//     a.config.Mail.FromEmail,
	// )
	notificationUseCase := usecase.NewNotificationUseCase(notificationRepo, emailSender)

	// Настраиваем RabbitMQ
	exchanges := map[string]string{
		"order_events":   "topic",
		"billing_events": "topic",
	}
	queues := map[string]map[string]string{
		"order_notification_queue": {
			"order_events": "order.#",
		},
		"billing_notification_queue": {
			"billing_events": "billing.#",
		},
	}

	if err := messaging.SetupExchangesAndQueues(a.rabbitMQ, exchanges, queues); err != nil {
		return errors.AppendPrefix(err, "ошибка при настройке RabbitMQ")
	}

	// Настраиваем обработчик сообщений
	err := a.rabbitMQ.ConsumeMessages("order_notification_queue", "notification-service", func(data []byte) error {
		return notificationUseCase.HandleOrderEvent(data)
	})
	if err != nil {
		return errors.AppendPrefix(err, "ошибка при настройке обработчика сообщений для заказов")
	}

	// Настраиваем обработчик сообщений от биллинга
	err = a.rabbitMQ.ConsumeMessages("billing_notification_queue", "notification-service-billing", func(data []byte) error {
		return notificationUseCase.HandleOrderEvent(data)
	})
	if err != nil {
		return errors.AppendPrefix(err, "ошибка при настройке обработчика сообщений для биллинга")
	}

	// Регистрируем HTTP обработчики
	notificationHandler := httpController.NewNotificationHandler(notificationUseCase)
	notificationHandler.RegisterRoutes(a.router)

	// Запускаем HTTP сервер в горутине
	go func() {
		log.Printf("HTTP сервер запущен на порту %s", a.config.HTTP.Port)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Ошибка запуска HTTP сервера: %v", err)
		}
	}()

	// Ожидаем сигнал завершения
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		log.Println("Получен сигнал завершения, закрываем приложение...")
	case <-ctx.Done():
		log.Println("Контекст завершен, закрываем приложение...")
	}

	return a.Shutdown()
}

// Shutdown корректно завершает работу приложения
func (a *App) Shutdown() error {
	errGroup := errors.NewErrorGroup()

	// Закрываем HTTP сервер
	if a.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := a.httpServer.Shutdown(ctx); err != nil {
			errGroup.AddPrefix(err, "ошибка при закрытии HTTP сервера")
		}
	}

	// Закрываем RabbitMQ
	if a.rabbitMQ != nil {
		a.rabbitMQ.Close()
	}

	// Закрываем соединение с базой данных
	if a.db != nil {
		if err := database.CloseDB(a.db); err != nil {
			errGroup.AddPrefix(err, "ошибка при закрытии соединения с базой данных")
		}
	}

	if errGroup.HasErrors() {
		errors.LogError(errGroup, "Shutdown")
		return errGroup
	}

	log.Println("Приложение успешно завершено")
	return nil
}
