# Reviews

Reviews — автономный Go-сервис, который собирает отзывы о товарах продавца с
маркетплейсов (Wildberries, Яндекс Маркет; Ozon — за флагом, требует платной
подписки), хранит их в локальной базе, даёт админ-панель для модерации и
настройки и отдаёт виджет с отзывами для встраивания на сайт.

Один бинарник делает всё: миграции БД, синхронизацию, HTTP-сервер, экспорт.

---

## Содержание

1. [Требования](#требования)
2. [Установка](#установка)
3. [Настройка (.env)](#настройка-env)
4. [Первый запуск](#первый-запуск)
5. [CLI-команды](#cli-команды)
6. [Админ-панель](#админ-панель)
7. [HTTP API](#http-api)
8. [Встраивание виджета](#встраивание-виджета)
9. [Обслуживание](#обслуживание)
10. [Чек-лист для продакшена](#чек-лист-для-продакшена)
11. [Документация по дизайну](#документация-по-дизайну)

---

## Требования

| Компонент | Версия | Зачем |
|-----------|--------|-------|
| Docker + Docker Compose | современный | рекомендуемый способ запуска |
| Go | 1.26+ | только для сборки из исходников |
| Node.js | 22+ | только для пересборки админ-панели (SPA) |
| SQLite | встроена | БД по умолчанию, отдельная установка не нужна |
| PostgreSQL | 13+ | опционально, вместо SQLite |

Ключи доступа к маркетплейсам (получаются в личном кабинете продавца):
- **Wildberries** — API-токен;
- **Яндекс Маркет** — `Api-Key` и `businessId`;
- **Ozon** — `Client-Id` и `Api-Key` (по умолчанию выключен).

---

## Установка

### Вариант A — Docker (рекомендуется)

```sh
git clone <repo-url> reviews
cd reviews

# 1. Подготовьте конфигурацию
cp .env.example .env
# отредактируйте .env (см. раздел «Настройка»)

# 2. Соберите и запустите
docker compose up -d --build

# 3. Проверьте, что сервис жив
curl http://localhost:8080/healthz
```

Что делает compose:
- собирает фронтенд (Vite) и Go-бинарник в multistage-образе (distroless, nonroot);
- запускает `serve --addr 0.0.0.0:8080 --with-sync` — сервер + встроенный
  планировщик синхронизации;
- хранит SQLite-базу на именованном томе `reviews-data` (`/data/reviews.db`),
  данные переживают пересоздание контейнера.

Управление:
```sh
docker compose logs -f reviews
docker compose restart reviews
docker compose down            # остановить (том с данными сохраняется)
```

### Вариант B — из исходников (без Docker)

```sh
git clone <repo-url> reviews
cd reviews
cp .env.example .env           # отредактируйте

# Собрать бинарник
go build -o reviews ./cmd/reviews

# Применить миграции БД
./reviews migrate

# Запустить сервер (локально, с синхронизацией)
./reviews serve --addr 127.0.0.1:8080 --with-sync
```

> **Локальный HTTP:** для запуска без HTTPS обязательно укажите в `.env`
> `REVIEWS_INSECURE_COOKIES=1`, иначе куки сессии получат флаг `Secure` и вход
> в админку не сработает. В продакшене за HTTPS — оставьте переменную пустой.

Если меняли код фронтенда — пересоберите SPA перед сборкой бинарника:
```sh
cd web/admin && npm ci && npm run build && cd ../..
# затем скопируйте dist в ассеты сервера согласно скриптам в deploy/
```

---

## Настройка (.env)

Все настройки задаются переменными окружения (или файлом `.env` в корне).
Полный пример — в [.env.example](.env.example).

### База данных
| Переменная | По умолчанию | Описание |
|-----------|--------------|----------|
| `REVIEWS_DB_DRIVER` | `sqlite` | `sqlite` или `postgres` |
| `REVIEWS_DB_DSN` | `./reviews.db` | путь к файлу SQLite или DSN Postgres. В Docker принудительно `/data/reviews.db` |

DSN для Postgres:
`host=db user=reviews password=secret dbname=reviews port=5432 sslmode=disable`

### Синхронизация (для `serve --with-sync` и `sync`)
| Переменная | По умолчанию | Описание |
|-----------|--------------|----------|
| `REVIEWS_SYNC_INTERVAL` | `1h` | период автосинхронизации |
| `REVIEWS_SYNC_BACKFILL_MONTHS` | `12` | глубина первичной загрузки (мес.) |
| `REVIEWS_SYNC_OVERLAP` | `1h` | перехлёст окна, чтобы не терять отзывы |

### Логи и безопасность
| Переменная | По умолчанию | Описание |
|-----------|--------------|----------|
| `REVIEWS_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `REVIEWS_LOG_FORMAT` | `json` | `json` или `text` |
| `REVIEWS_INSECURE_COOKIES` | — | `1` → разрешить куки по HTTP (только локально). За HTTPS оставьте пустым |

### Сайт / виджет
| Переменная | По умолчанию | Описание |
|-----------|--------------|----------|
| `REVIEWS_SITE_PRODUCT_URL_TEMPLATE` | — | шаблон ссылки на товар, `{seller_article_url}` подставляется |
| `REVIEWS_SITE_PRODUCT_LINKS` | `data/product-links.json` | файл с точными ссылками на товары (приоритетнее шаблона) |

### Маркетплейсы
Каждый включается флагом `*_ENABLED=true` **после** заполнения ключей.

| Wildberries | Яндекс Маркет | Ozon (выключен) |
|-------------|---------------|-----------------|
| `REVIEWS_WB_ENABLED` | `REVIEWS_YM_ENABLED` | `REVIEWS_OZON_ENABLED` |
| `REVIEWS_WB_TOKEN` | `REVIEWS_YM_API_KEY` | `REVIEWS_OZON_CLIENT_ID` |
| | `REVIEWS_YM_BUSINESS_ID` | `REVIEWS_OZON_API_KEY` |
| | `REVIEWS_YM_CAMPAIGN_ID` (опц.) | |

---

## Первый запуск

1. **Применить миграции** (Docker делает это сам при старте):
   ```sh
   ./reviews migrate
   ```
2. **Запустить сервер:**
   ```sh
   ./reviews serve --with-sync         # или docker compose up -d
   ```
3. **Создать администратора.** Откройте `http://localhost:8080/admin` — при
   первом запуске SPA покажет экран **первичной настройки**. Задайте логин и
   пароль (минимум 8 символов). Повторно создать админа через этот экран
   нельзя — эндпоинт вернёт 409, если админ уже существует.

   Альтернатива / сброс пароля через CLI:
   ```sh
   ./reviews admin reset-password --login admin --password НОВЫЙ_ПАРОЛЬ
   ```
   (сбрасывает пароль и завершает все активные сессии пользователя)

4. **Войти** на `/admin` под созданными логином и паролем.

---

## CLI-команды

```sh
reviews migrate
    Применить миграции схемы БД.

reviews admin reset-password --login admin --password NEW
    Сменить пароль администратора и разлогинить его сессии.

reviews sync --once [--marketplace wb|ym|ozon]
    Разовая синхронизация. Без --marketplace — все включённые.

reviews serve [--addr 127.0.0.1:8080] [--with-sync]
              [--static-dir web/reviews-widget]
              [--product-url-template "..."]
    HTTP-сервер. --with-sync включает фоновый планировщик.

reviews discover-site-urls [--sitemap URL] [--out data/...json]
    Сопоставить SKU продавца с точными ссылками из sitemap сайта.

reviews export [--out web/reviews-data]
    Выгрузить отзывы в статические JSON-бандлы (для CDN/статик-хостинга).
```

---

## Админ-панель

Доступна на `/admin` после входа. Основные разделы:

- **Дашборд** — сводка: количество отзывов, статусы, состояние синхронизации
  по маркетплейсам.
- **Отзывы** — список с фильтрами, модерация: одобрение/скрытие отзыва.
- **Маркетплейсы** — статус подключения (включён / настроен) и кнопка ручного
  запуска синхронизации.
- **Витрина (Showcase)** — правило отбора отзывов для виджета (рейтинг, наличие
  фото и т.п.).
- **Редактор виджета (Editor)** — визуальная настройка темы, раскладки,
  типографики и видимости элементов с живым предпросмотром в iframe. Публикация
  версионная, есть откат к предыдущей версии.
- **Встраивание (Embed)** — готовый сниппет для вставки виджета на сайт.

Все изменяющие запросы защищены сессией и CSRF-токеном (double-submit cookie).

---

## HTTP API

### Публичные (без авторизации)
| Метод | Путь | Назначение |
|-------|------|-----------|
| GET | `/api/reviews` | отзывы (фильтры: `marketplace`, `rating`, `offset`) |
| GET | `/api/showcase` | отзывы для витрины по правилу showcase |
| GET | `/api/widget-config` | опубликованная конфигурация виджета |
| GET | `/healthz` | health-check |
| GET | `/` | статика виджета (`--static-dir`) |

### Админские (`/admin/api/`, требуют сессию + CSRF на запись)
`setup-status`, `setup`, `login`, `logout`, `me`, `csrf`, `reviews`,
`reviews/{id}` (PATCH), `dashboard`, `marketplaces`, `sync` (POST),
`showcase-rule` (GET/PUT), `widget-config/{context}` (GET/POST),
`widget-config/{context}/versions`, `widget-config/{context}/rollback/{version}`.

---

## Встраивание виджета

Продакшен-путь — статический (быстрый, без нагрузки на сервис):

1. Выгрузить данные в JSON-бандлы:
   ```sh
   reviews export --out web/reviews-data
   ```
2. Раздавать по HTTPS файлы `loader.js`, `reviews-widget.js`,
   `reviews-widget.css` и папку `reviews-data/` (Caddy/Nginx или CDN).
3. Подключить `loader.js` на странице товара (можно через Яндекс Тег Менеджер:
   Custom HTML, на всех страницах / DOM Ready).
4. `loader.js` сам отслеживает SPA-навигацию, достаёт SKU товара со страницы,
   подгружает `reviews-data/by-article/<sku>.json`, монтирует виджет в Shadow
   DOM после блока с товаром и добавляет JSON-LD-разметку для SEO.

Альтернативно виджет может ходить напрямую в `GET /api/reviews` живого сервиса
(динамический режим) — удобно для теста, см.
[web/reviews-widget/demo.html](web/reviews-widget/demo.html).

---

## Обслуживание

**Бэкап БД (SQLite):**
```sh
docker compose exec reviews sh -c 'cp /data/reviews.db /data/reviews.backup.db'
# или копировать том reviews-data целиком
```

**Ручная синхронизация** — из админки (кнопка) или CLI:
```sh
./reviews sync --once --marketplace ym
```

**Логи** — структурированные (JSON по умолчанию):
```sh
docker compose logs -f reviews
```

---

## Чек-лист для продакшена

- [ ] Сервис за HTTPS (reverse proxy: Caddy/Nginx), `REVIEWS_INSECURE_COOKIES`
      **пустой** (куки `Secure`).
- [ ] Заполнены и включены ключи нужных маркетплейсов (`*_ENABLED=true`).
- [ ] Создан администратор с надёжным паролем.
- [ ] Том с БД (`reviews-data`) включён в регулярный бэкап.
- [ ] Проверен `GET /healthz` и первый прогон `sync --once`.
- [ ] При раздельной раздаче виджета — настроен `export` (по cron/CI) и
      HTTPS-хостинг статики.

---

## Документация по дизайну

- [Дизайн коллектора (Phase 1)](docs/superpowers/specs/2026-06-04-reviews-collector-design.md)
- [План реализации](docs/superpowers/specs/2026-06-04-reviews-collector-implementation-plan.md)
- [Operations runbook](docs/superpowers/specs/2026-06-04-reviews-collector-runbook.md)
- [Виджет: прототип рендеринга](docs/superpowers/specs/2026-06-04-reviews-widget-rendering.md)
- [Встраивание на сайт: дизайн](docs/superpowers/specs/2026-06-08-reviews-site-embedding-design.md)
- [Встраивание на сайт: план](docs/superpowers/plans/2026-06-08-reviews-site-embedding.md)
- Деплой и CI/CD: [deploy/](deploy/), [.github/workflows](.github/workflows)
