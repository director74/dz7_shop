# Интернет-магазин на базе микросервисов Go

Проект представляет собой реализацию микросервисной архитектуры с использованием Go, Gin, GORM и RabbitMQ.

## Сервисы

Проект состоит из трех микросервисов:

1. **Сервис заказов** - управление пользователями и заказами
2. **Сервис биллинга** - управление счетами пользователей и транзакциями
3. **Сервис нотификаций** - отправка и хранение уведомлений

## Взаимодействие между сервисами

- При создании пользователя в **сервисе заказов** автоматически создается аккаунт в **сервисе биллинга**
- При создании заказа в **сервисе заказов**:
  1. Происходит списание средств через **сервис биллинга**
  2. Отправляется событие в RabbitMQ
  3. **Сервис нотификаций** получает событие и отправляет соответствующее уведомление
- **Единая аутентификация** между сервисами:
  1. JWT токен, полученный в любом сервисе, работает во всех сервисах системы
  2. Единый ключ подписи JWT и общие настройки обеспечивают бесшовную аутентификацию

## Технологии

- **Go** - язык программирования
- **Gin** - HTTP фреймворк
- **GORM** - ORM для работы с базой данных
- **PostgreSQL** - база данных
- **RabbitMQ** - брокер сообщений
- **Docker / Docker Compose** - контейнеризация и оркестрация

## Архитектура

Проект основан на принципах Clean Architecture:

- **Entity** - бизнес-модели
- **UseCase** - бизнес-логика
- **Repository** - работа с данными
- **Controller** - обработка запросов

### Особенности архитектуры

- **Общие компоненты** в директории `pkg/` для повторного использования в разных сервисах
- **Единая система аутентификации** на базе JWT с общим ключом для всех сервисов
- **Асинхронное взаимодействие** через RabbitMQ для обеспечения слабой связанности сервисов

## Запуск проекта

### Предварительные требования

- Docker
- Docker Compose

### Запуск

```bash
# Клонировать репозиторий
git clone https://github.com/director74/dz7_shop.git
cd dz7_shop/src

# Запустить проект
docker-compose -f deployments/docker-compose.yml up -d
```

## E2E тестирование в Postman

Для полного тестирования взаимодействия между микросервисами создана коллекция тестов Postman, автоматизирующая следующий сценарий:

1. Регистрация пользователя
2. Авторизация пользователя
3. Проверка автоматического создания аккаунта в биллинге
4. Пополнение баланса
5. Создание заказа, на который хватает денег
6. Проверка, что с баланса списаны средства
7. Проверка, что отправлено уведомление об успешном заказе
8. Создание заказа, на который не хватает денег
9. Проверка, что баланс не изменился
10. Проверка, что отправлено уведомление о проблеме с заказом

Коллекция тестов использует переменные для хранения промежуточных данных (ID пользователя, токен, баланс и т.д.) и автоматически проверяет результаты каждого шага.

### Как импортировать и запустить тесты:

1. Скачайте файл коллекции из репозитория (папка `tests`)
2. В Postman нажмите кнопку Import и выберите скачанный файл
3. Запустите все сервисы проекта с помощью Docker Compose
4. В Postman выберите коллекцию и нажмите "Run"
5. Просмотрите результаты выполнения всех тестов

## Структура проекта

```
src/
├── billing-service/       # Сервис биллинга
├── order-service/         # Сервис заказов
├── notification-service/  # Сервис нотификаций
├── pkg/                   # Общие пакеты
├── migrations/            # Миграции баз данных
├── deployments/           # Конфигурация Docker Compose
├── build/                 # Скрипты сборки
├── tests/                 # Тесты Postman
└── README.md              # Документация
```

## Документация

### API Спецификация (Swagger)

API-интерфейсы всех сервисов документированы с использованием OpenAPI (Swagger).
Полную спецификацию можно найти в файле [swagger.yaml](docs/swagger.yaml).

### Диаграмма последовательности взаимодействия

Для визуализации взаимодействия между микросервисами создана диаграмма последовательности в формате PlantUML.
Диаграмма показывает основные сценарии: регистрацию, авторизацию, создание заказов, пополнение счета.
См. [sequence_diagram.plantuml](docs/sequence_diagram.plantuml)

## Мониторинг

- RabbitMQ Management: http://localhost:15672 (guest/guest)
- MailHog (для просмотра отправленных писем): http://localhost:8025 

## API Методы

### Сервис заказов (порт 8080)

#### Основные
- **GET** `/health` - Проверка состояния сервиса
- **POST** `/api/v1/users` - Создание пользователя (публичный эндпоинт)

#### Аутентификация
- **POST** `/api/v1/auth/register` - Регистрация нового пользователя
- **POST** `/api/v1/auth/login` - Вход пользователя

#### Заказы (требуется аутентификация)
- **POST** `/api/v1/orders` - Создание заказа
- **GET** `/api/v1/orders/:id` - Получение заказа по ID
- **GET** `/api/v1/users/:id/orders` - Получение списка заказов пользователя

### Сервис биллинга (порт 8081)

#### Основные
- **GET** `/health` - Проверка состояния сервиса
- **POST** `/api/v1/accounts` - Создание аккаунта пользователя
- **GET** `/api/v1/accounts/:user_id` - Получение аккаунта пользователя по ID

#### Операции с аккаунтом (требуется аутентификация)
- **GET** `/api/v1/billing/account` - Получение информации о своем аккаунте
- **POST** `/api/v1/billing/deposit` - Пополнение баланса своего аккаунта
- **POST** `/api/v1/billing/withdraw` - Списание средств со своего аккаунта

### Сервис нотификаций (порт 8082)

#### Основные
- **GET** `/health` - Проверка состояния сервиса
- **POST** `/api/v1/notifications` - Отправка уведомления
- **GET** `/api/v1/notifications/:id` - Получение уведомления по ID
- **GET** `/api/v1/users/:id/notifications` - Получение списка уведомлений пользователя
- **GET** `/api/v1/notifications` - Получение списка всех уведомлений 

## Визуальные материалы

### Диаграмма последовательности взаимодействия

Диаграмма последовательности доступна в виде SVG-изображения: ![Открыть диаграмму](docs/sequence_diagramm.svg) 