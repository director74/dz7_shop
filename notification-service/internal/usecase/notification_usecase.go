package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/director74/dz7_shop/notification-service/internal/entity"
)

// NotificationRepository интерфейс для работы с хранилищем нотификаций
type NotificationRepository interface {
	CreateNotification(ctx context.Context, notification entity.Notification) (entity.Notification, error)
	GetNotificationByID(ctx context.Context, id uint) (entity.Notification, error)
	UpdateNotificationStatus(ctx context.Context, id uint, status string) error
	ListNotificationsByUserID(ctx context.Context, userID uint, limit, offset int) ([]entity.Notification, int64, error)
	ListAllNotifications(ctx context.Context, limit, offset int) ([]entity.Notification, int64, error)
}

// EmailSender интерфейс для отправки электронной почты
type EmailSender interface {
	SendEmail(to, subject, message string) error
}

// NotificationUseCase представляет usecase для работы с нотификациями
type NotificationUseCase struct {
	repo        NotificationRepository
	emailSender EmailSender
}

func NewNotificationUseCase(repo NotificationRepository, emailSender EmailSender) *NotificationUseCase {
	return &NotificationUseCase{
		repo:        repo,
		emailSender: emailSender,
	}
}

func (uc *NotificationUseCase) SendNotification(ctx context.Context, req entity.SendNotificationRequest) (entity.SendNotificationResponse, error) {
	notification := entity.Notification{
		UserID:    req.UserID,
		Email:     req.Email,
		Subject:   req.Subject,
		Message:   req.Message,
		Status:    entity.NotificationStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	newNotification, err := uc.repo.CreateNotification(ctx, notification)
	if err != nil {
		return entity.SendNotificationResponse{}, fmt.Errorf("ошибка при создании уведомления: %w", err)
	}

	// В реальном приложении здесь была бы отправка почты
	// В нашем случае, мы просто меняем статус на "sent"
	// err = uc.emailSender.SendEmail(req.Email, req.Subject, req.Message)
	// if err != nil {
	//     _ = uc.repo.UpdateNotificationStatus(ctx, newNotification.ID, entity.NotificationStatusFailed)
	//     return entity.SendNotificationResponse{}, fmt.Errorf("ошибка при отправке уведомления: %w", err)
	// }

	err = uc.repo.UpdateNotificationStatus(ctx, newNotification.ID, entity.NotificationStatusSent)
	if err != nil {
		return entity.SendNotificationResponse{}, fmt.Errorf("ошибка при обновлении статуса уведомления: %w", err)
	}

	return entity.SendNotificationResponse{
		ID:      newNotification.ID,
		UserID:  newNotification.UserID,
		Email:   newNotification.Email,
		Subject: newNotification.Subject,
		Status:  entity.NotificationStatusSent,
	}, nil
}

func (uc *NotificationUseCase) ProcessOrderNotification(ctx context.Context, orderNotification entity.OrderNotification) error {
	var subject, message string

	if orderNotification.Success {
		subject = fmt.Sprintf("Заказ #%d успешно оформлен", orderNotification.OrderID)
		message = fmt.Sprintf("Уважаемый клиент, ваш заказ #%d на сумму %.2f успешно оформлен. Спасибо за покупку!",
			orderNotification.OrderID, orderNotification.Amount)
	} else {
		subject = fmt.Sprintf("Проблема с заказом #%d", orderNotification.OrderID)
		message = fmt.Sprintf("Уважаемый клиент, при оформлении заказа #%d на сумму %.2f возникла проблема. Пожалуйста, проверьте баланс вашего счета.",
			orderNotification.OrderID, orderNotification.Amount)
	}

	req := entity.SendNotificationRequest{
		UserID:  orderNotification.UserID,
		Email:   orderNotification.Email,
		Subject: subject,
		Message: message,
	}

	_, err := uc.SendNotification(ctx, req)
	return err
}

func (uc *NotificationUseCase) ProcessDepositNotification(ctx context.Context, depositNotification entity.DepositNotification) error {
	// Используем email из сообщения или формируем заглушку
	email := depositNotification.Email
	if email == "" {
		email = fmt.Sprintf("user%d@example.com", depositNotification.UserID)
	}

	subject := "Пополнение баланса"
	message := fmt.Sprintf("Уважаемый клиент, ваш счет был пополнен на сумму %.2f. Текущая операция: %s.",
		depositNotification.Amount, depositNotification.OperationType)

	req := entity.SendNotificationRequest{
		UserID:  depositNotification.UserID,
		Email:   email,
		Subject: subject,
		Message: message,
	}

	_, err := uc.SendNotification(ctx, req)
	return err
}

func (uc *NotificationUseCase) ProcessInsufficientFundsNotification(ctx context.Context, notification entity.InsufficientFundsNotification) error {
	// Используем email из уведомления
	email := notification.Email

	// Если email пустой, используем заглушку (для обратной совместимости)
	if email == "" {
		email = fmt.Sprintf("user%d@example.com", notification.UserID)
	}

	subject := "Недостаточно средств на вашем счете"
	message := fmt.Sprintf("Уважаемый клиент, на вашем счете недостаточно средств для совершения операции на сумму %.2f. "+
		"Текущий баланс: %.2f. Пожалуйста, пополните баланс для совершения покупок.",
		notification.Amount, notification.Balance)

	req := entity.SendNotificationRequest{
		UserID:  notification.UserID,
		Email:   email,
		Subject: subject,
		Message: message,
	}

	_, err := uc.SendNotification(ctx, req)
	return err
}

func (uc *NotificationUseCase) GetNotification(ctx context.Context, id uint) (entity.GetNotificationResponse, error) {
	notification, err := uc.repo.GetNotificationByID(ctx, id)
	if err != nil {
		return entity.GetNotificationResponse{}, fmt.Errorf("уведомление не найдено: %w", err)
	}

	return entity.GetNotificationResponse{
		ID:        notification.ID,
		UserID:    notification.UserID,
		Email:     notification.Email,
		Subject:   notification.Subject,
		Message:   notification.Message,
		Status:    notification.Status,
		CreatedAt: notification.CreatedAt,
	}, nil
}

func (uc *NotificationUseCase) ListUserNotifications(ctx context.Context, userID uint, limit, offset int) (entity.ListNotificationsResponse, error) {
	notifications, total, err := uc.repo.ListNotificationsByUserID(ctx, userID, limit, offset)
	if err != nil {
		return entity.ListNotificationsResponse{}, fmt.Errorf("ошибка при получении списка уведомлений: %w", err)
	}

	var response entity.ListNotificationsResponse
	response.Total = total
	response.Notifications = make([]entity.GetNotificationResponse, len(notifications))

	for i, notification := range notifications {
		response.Notifications[i] = entity.GetNotificationResponse{
			ID:        notification.ID,
			UserID:    notification.UserID,
			Email:     notification.Email,
			Subject:   notification.Subject,
			Message:   notification.Message,
			Status:    notification.Status,
			CreatedAt: notification.CreatedAt,
		}
	}

	return response, nil
}

func (uc *NotificationUseCase) ListAllNotifications(ctx context.Context, limit, offset int) (entity.ListNotificationsResponse, error) {
	notifications, total, err := uc.repo.ListAllNotifications(ctx, limit, offset)
	if err != nil {
		return entity.ListNotificationsResponse{}, fmt.Errorf("ошибка при получении списка уведомлений: %w", err)
	}

	var response entity.ListNotificationsResponse
	response.Total = total
	response.Notifications = make([]entity.GetNotificationResponse, len(notifications))

	for i, notification := range notifications {
		response.Notifications[i] = entity.GetNotificationResponse{
			ID:        notification.ID,
			UserID:    notification.UserID,
			Email:     notification.Email,
			Subject:   notification.Subject,
			Message:   notification.Message,
			Status:    notification.Status,
			CreatedAt: notification.CreatedAt,
		}
	}

	return response, nil
}

// HandleOrderEvent обрабатывает событие заказа из RabbitMQ
func (uc *NotificationUseCase) HandleOrderEvent(data []byte) error {
	log.Printf("Получено событие заказа: %s", string(data))

	// Сначала пытаемся распознать тип события
	var baseEvent struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(data, &baseEvent); err != nil {
		return fmt.Errorf("ошибка при парсинге базового события: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// В зависимости от типа события, обрабатываем его
	switch {
	case baseEvent.Type == "order.created" || baseEvent.Type == "":
		// Событие создания заказа или без указанного типа (для обратной совместимости)
		var orderEvent entity.OrderNotification
		if err := json.Unmarshal(data, &orderEvent); err != nil {
			return fmt.Errorf("ошибка при парсинге события создания заказа: %w", err)
		}
		return uc.ProcessOrderNotification(ctx, orderEvent)

	case baseEvent.Type == "billing.deposit":
		// Событие пополнения баланса
		var depositEvent entity.DepositNotification
		if err := json.Unmarshal(data, &depositEvent); err != nil {
			return fmt.Errorf("ошибка при парсинге события пополнения баланса: %w", err)
		}
		return uc.ProcessDepositNotification(ctx, depositEvent)

	case baseEvent.Type == "billing.insufficient_funds":
		// Событие недостатка средств
		var insufficientEvent entity.InsufficientFundsNotification
		if err := json.Unmarshal(data, &insufficientEvent); err != nil {
			return fmt.Errorf("ошибка при парсинге события недостатка средств: %w", err)
		}
		return uc.ProcessInsufficientFundsNotification(ctx, insufficientEvent)

	default:
		log.Printf("Неизвестный тип события: %s, игнорируем", baseEvent.Type)
		return nil
	}
}
