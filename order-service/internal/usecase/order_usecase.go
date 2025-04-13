package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/director74/dz7_shop/order-service/internal/entity"
	"github.com/director74/dz7_shop/order-service/internal/repo"
)

// OrderUseCase представляет usecase для работы с заказами
type OrderUseCase struct {
	repo      repo.OrderRepository
	userRepo  repo.UserRepository
	billing   BillingService
	rabbitMQ  RabbitMQClient
	orderExch string
}

func NewOrderUseCase(orderRepo repo.OrderRepository, userRepo repo.UserRepository, billing BillingService, rabbitMQ RabbitMQClient, orderExch string) *OrderUseCase {
	return &OrderUseCase{
		repo:      orderRepo,
		userRepo:  userRepo,
		billing:   billing,
		rabbitMQ:  rabbitMQ,
		orderExch: orderExch,
	}
}

func (uc *OrderUseCase) CreateUser(ctx context.Context, req entity.CreateUserRequest) (entity.CreateUserResponse, error) {
	_, err := uc.userRepo.GetByEmail(ctx, req.Email)
	if err == nil {
		return entity.CreateUserResponse{}, errors.New("пользователь с таким email уже существует")
	}

	user := &entity.User{
		Username:  req.Username,
		Email:     req.Email,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = uc.userRepo.Create(ctx, user)
	if err != nil {
		return entity.CreateUserResponse{}, fmt.Errorf("ошибка при создании пользователя: %w", err)
	}

	err = uc.billing.CreateAccount(ctx, user.ID)
	if err != nil {
		// При ошибке создания аккаунта в биллинге удаляем пользователя
		deleteErr := uc.userRepo.Delete(ctx, user.ID)
		if deleteErr != nil {
			// Логируем ошибку удаления, но возвращаем основную ошибку
			fmt.Printf("Ошибка при удалении пользователя после неудачного создания аккаунта в биллинге: %v\n", deleteErr)
		}
		return entity.CreateUserResponse{}, fmt.Errorf("ошибка при создании аккаунта в биллинге: %w", err)
	}

	return entity.CreateUserResponse{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
	}, nil
}

func (uc *OrderUseCase) CreateOrder(ctx context.Context, req entity.CreateOrderRequest) (entity.CreateOrderResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	user, err := uc.userRepo.GetByID(ctx, req.UserID)
	if err != nil {
		return entity.CreateOrderResponse{}, fmt.Errorf("пользователь не найден: %w", err)
	}

	// Получаем JWT токен из контекста запроса
	token := ""
	if tokenValue := ctx.Value("jwt_token"); tokenValue != nil {
		if tokenStr, ok := tokenValue.(string); ok {
			token = tokenStr
		}
	}

	// Пытаемся снять деньги с аккаунта пользователя
	success, err := uc.billing.WithdrawMoney(ctx, req.UserID, req.Amount, user.Email, token)
	if err != nil {
		return entity.CreateOrderResponse{}, fmt.Errorf("ошибка при списании средств: %w", err)
	}

	// Определяем статус заказа на основе результата списания
	status := entity.OrderStatusFailed
	if success {
		status = entity.OrderStatusCompleted
	}

	// Создаем заказ с окончательным статусом
	order := &entity.Order{
		UserID:    req.UserID,
		Amount:    req.Amount,
		Status:    status,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = uc.repo.Create(ctx, order)
	if err != nil {
		// Если не удалось создать заказ, но деньги были списаны, нужно их вернуть
		// Это потребует дополнительного метода в BillingService
		// Здесь мы просто логируем проблему
		if success {
			log.Printf("КРИТИЧЕСКАЯ ОШИБКА: Деньги были списаны, но заказ не был создан: userID=%d, amount=%.2f, error=%v",
				req.UserID, req.Amount, err)
		}
		return entity.CreateOrderResponse{}, fmt.Errorf("ошибка при создании заказа: %w", err)
	}

	// Отправляем событие в RabbitMQ для нотификации о заказе (успешном или нет)
	notification := struct {
		UserID  uint    `json:"user_id"`
		Email   string  `json:"email"`
		OrderID uint    `json:"order_id"`
		Amount  float64 `json:"amount"`
		Success bool    `json:"success"`
	}{
		UserID:  user.ID,
		Email:   user.Email,
		OrderID: order.ID,
		Amount:  order.Amount,
		Success: success,
	}

	// Используем метод с повторными попытками для надежной публикации
	err = uc.rabbitMQ.PublishMessageWithRetry(uc.orderExch, "order.notification", notification, 3)
	if err != nil {
		// Логируем ошибку, но не прерываем выполнение
		log.Printf("Ошибка при отправке нотификации после %d попыток: %v\n", 3, err)
	}

	return entity.CreateOrderResponse{
		ID:     order.ID,
		UserID: order.UserID,
		Amount: order.Amount,
		Status: status,
	}, nil
}

func (uc *OrderUseCase) GetOrder(ctx context.Context, id uint) (entity.GetOrderResponse, error) {
	order, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return entity.GetOrderResponse{}, fmt.Errorf("заказ не найден: %w", err)
	}

	return entity.GetOrderResponse{
		ID:        order.ID,
		UserID:    order.UserID,
		Amount:    order.Amount,
		Status:    order.Status,
		CreatedAt: order.CreatedAt,
		UpdatedAt: order.UpdatedAt,
	}, nil
}

func (uc *OrderUseCase) ListUserOrders(ctx context.Context, userID uint, limit, offset int) (entity.ListOrdersResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	orders, err := uc.repo.GetByUserID(ctx, userID, limit, offset)
	if err != nil {
		return entity.ListOrdersResponse{}, fmt.Errorf("ошибка при получении списка заказов: %w", err)
	}

	total, err := uc.repo.CountByUserID(ctx, userID)
	if err != nil {
		return entity.ListOrdersResponse{}, fmt.Errorf("ошибка при получении общего количества заказов: %w", err)
	}

	var response entity.ListOrdersResponse
	response.Total = total
	response.Orders = make([]entity.GetOrderResponse, len(orders))

	for i, order := range orders {
		response.Orders[i] = entity.GetOrderResponse{
			ID:        order.ID,
			UserID:    order.UserID,
			Amount:    order.Amount,
			Status:    order.Status,
			CreatedAt: order.CreatedAt,
			UpdatedAt: order.UpdatedAt,
		}
	}

	return response, nil
}
