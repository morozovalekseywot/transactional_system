Нужно реализовать транзакционную систему. Сообщения отправляются и принимаются через брокера сообщений.
- Invoice -> человеку зачисляются средства по ручке "/invoice" с такими параметрами в теле, как:
    ````Golang
    type InvoiceRequest struct {
        WalletID int     `json:"wallet_id"` // код валюты ("USDT", "RUB", "EUR", etc.)
        Ticker   string  `json:"ticker"` // номер кошелька или карты
        Amount   float32 `json:"amount"` // количество средств (число с плавающей точкой)
    }
   ````
- Withdraw -> человек выводит средства со своего баланса по валюте, которую он выбрал по ручке "/withdraw" с такими параметрами в теле, как:
  ````Golang
    type WithdrawRequest struct {
        WalletID int     `json:"wallet_id"` // номер кошелька или карты куда зачисляются средства
        Ticker   string  `json:"ticker"` // код валюты
        Amount   float32 `json:"amount"` // количество средств для списания
    }
   ````
- В транзакционной системе должны быть статусы транзакции ("Error", "Success", "Created"). Статусы "Error" и "Success" должны быть финальными.
- Должна быть реализована ручка balance -> по получению актуального и замороженного баланса клиентов. 
  Актуальный баланс это тот баланс, который можно вывести. Замороженный баланс - это тот баланс, который, находится в ожидании (со статусом "Created").
  ````Golang
    type GetBalanceRequest struct {
        WalletID int `json:"wallet_id"` // номер кошелька или карты
    }
    
    type GetBalanceResponse struct {
        ActualBalance map[string]float32 `json:"actual_balance,omitempty"`
        FrozenBalance map[string]float32 `json:"frozen_balance,omitempty"`
    }
   ````
- В качестве брокера сообщений использован RabbitMQ.
- В качестве базы данных использована PostgreSQL.
- Баланс клиента не может уйти ниже нуля.

Так же было добавлено снятие метрик с помощью Prometheus. Для каждой из ручек подсчитывается количество статусов ответа.

Для тестирования был добавлен модуль internal/helpers. В нём реализовано заполнение бд тестовыми данными и запуск
клиента, который будет отправлять запросы через брокера сообщений и ждать ответ.

### Детали:
Сообщение в брокере разделяются по routingKey, он должен быть равен типу операции. При получении сообщения проверяется
корректность полей. Например, то что amount при зачислении указан больше 0, и то что указан существующий 
кошелёк и тикер. Вида ответа может быть два:
  ```Golang
    type ErrorResponse struct {
      Operation string `json:"operation"`
      Code      int    `json:"code"`
      Reason    string `json:"reason"`
    }
  
    type SuccessResponse struct {
        Operation string `json:"operation"`
        Code      int    `json:"code"`
        Body      string `json:"body,omitempty"`
    }

  ```