package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/director74/dz7_shop/notification-service/internal/entity"
	"github.com/director74/dz7_shop/notification-service/internal/usecase"
	"github.com/director74/dz7_shop/pkg/rabbitmq"
)

type NotificationConsumer struct {
	notificationUseCase *usecase.NotificationUseCase
	rabbitMQ            *rabbitmq.RabbitMQ
}

func NewNotificationConsumer(notificationUseCase *usecase.NotificationUseCase, rabbitMQ *rabbitmq.RabbitMQ) *NotificationConsumer {
	return &NotificationConsumer{
		notificationUseCase: notificationUseCase,
		rabbitMQ:            rabbitMQ,
	}
}

// Setup настраивает обработчик событий
func (c *NotificationConsumer) Setup(orderExch string) error {
	// Объявляем exchange для заказов
	err := c.rabbitMQ.DeclareExchange(orderExch, "topic")
	if err != nil {
		return fmt.Errorf("ошибка при объявлении exchange для заказов: %w", err)
	}

	// Объявляем exchange для биллинга
	err = c.rabbitMQ.DeclareExchange("billing_events", "topic")
	if err != nil {
		return fmt.Errorf("ошибка при объявлении exchange для биллинга: %w", err)
	}

	// Объявляем очередь для заказов
	orderQueue, err := c.rabbitMQ.DeclareQueueWithReturn("order_notifications")
	if err != nil {
		return fmt.Errorf("ошибка при объявлении очереди заказов: %w", err)
	}

	// Привязываем очередь заказов к exchange с ключом
	err = c.rabbitMQ.BindQueue(orderQueue.Name, orderExch, "order.notification")
	if err != nil {
		return fmt.Errorf("ошибка при привязке очереди заказов к exchange: %w", err)
	}

	// Объявляем очередь для пополнений баланса
	depositQueue, err := c.rabbitMQ.DeclareQueueWithReturn("deposit_notifications")
	if err != nil {
		return fmt.Errorf("ошибка при объявлении очереди пополнений: %w", err)
	}

	// Привязываем очередь пополнений к exchange с ключом
	err = c.rabbitMQ.BindQueue(depositQueue.Name, "billing_events", "billing.deposit")
	if err != nil {
		return fmt.Errorf("ошибка при привязке очереди пополнений к exchange: %w", err)
	}

	// Объявляем очередь для уведомлений о недостатке средств
	insufficientFundsQueue, err := c.rabbitMQ.DeclareQueueWithReturn("insufficient_funds_notifications")
	if err != nil {
		return fmt.Errorf("ошибка при объявлении очереди недостатка средств: %w", err)
	}

	// Привязываем очередь недостатка средств к exchange с ключом
	err = c.rabbitMQ.BindQueue(insufficientFundsQueue.Name, "billing_events", "billing.insufficient_funds")
	if err != nil {
		return fmt.Errorf("ошибка при привязке очереди недостатка средств к exchange: %w", err)
	}

	return nil
}

// StartConsuming начинает обработку сообщений
func (c *NotificationConsumer) StartConsuming() error {
	err := c.rabbitMQ.ConsumeMessages("order_notifications", "notification_service_orders", c.handleOrderNotification)
	if err != nil {
		return fmt.Errorf("ошибка при начале обработки сообщений заказов: %w", err)
	}

	err = c.rabbitMQ.ConsumeMessages("deposit_notifications", "notification_service_deposits", c.handleDepositNotification)
	if err != nil {
		return fmt.Errorf("ошибка при начале обработки сообщений пополнений: %w", err)
	}

	err = c.rabbitMQ.ConsumeMessages("insufficient_funds_notifications", "notification_service_insufficient_funds", c.handleInsufficientFundsNotification)
	if err != nil {
		return fmt.Errorf("ошибка при начале обработки сообщений о недостатке средств: %w", err)
	}

	return nil
}

// handleOrderNotification обрабатывает уведомление о заказе
func (c *NotificationConsumer) handleOrderNotification(body []byte) error {
	var orderNotification entity.OrderNotification

	err := json.Unmarshal(body, &orderNotification)
	if err != nil {
		return fmt.Errorf("ошибка при десериализации сообщения о заказе: %w", err)
	}

	log.Printf("Получено уведомление о заказе: %+v", orderNotification)

	err = c.notificationUseCase.ProcessOrderNotification(context.Background(), orderNotification)
	if err != nil {
		return fmt.Errorf("ошибка при обработке уведомления о заказе: %w", err)
	}

	log.Printf("Уведомление о заказе успешно обработано")
	return nil
}

// handleDepositNotification обрабатывает уведомление о пополнении баланса
func (c *NotificationConsumer) handleDepositNotification(body []byte) error {
	var depositNotification entity.DepositNotification

	err := json.Unmarshal(body, &depositNotification)
	if err != nil {
		return fmt.Errorf("ошибка при десериализации сообщения о пополнении: %w", err)
	}

	log.Printf("Получено уведомление о пополнении баланса: %+v", depositNotification)

	err = c.notificationUseCase.ProcessDepositNotification(context.Background(), depositNotification)
	if err != nil {
		return fmt.Errorf("ошибка при обработке уведомления о пополнении: %w", err)
	}

	log.Printf("Уведомление о пополнении баланса успешно обработано")
	return nil
}

// handleInsufficientFundsNotification обрабатывает уведомление о недостатке средств
func (c *NotificationConsumer) handleInsufficientFundsNotification(body []byte) error {
	var insufficientFundsNotification entity.InsufficientFundsNotification

	err := json.Unmarshal(body, &insufficientFundsNotification)
	if err != nil {
		return fmt.Errorf("ошибка при десериализации сообщения о недостатке средств: %w", err)
	}

	log.Printf("Получено уведомление о недостатке средств: %+v", insufficientFundsNotification)

	err = c.notificationUseCase.ProcessInsufficientFundsNotification(context.Background(), insufficientFundsNotification)
	if err != nil {
		return fmt.Errorf("ошибка при обработке уведомления о недостатке средств: %w", err)
	}

	log.Printf("Уведомление о недостатке средств успешно обработано")
	return nil
}
