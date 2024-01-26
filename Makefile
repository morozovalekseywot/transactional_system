build:
	docker-compose build app

run:
	docker-compose up app

migrate:
	migrate -path ./migrations -database 'postgresql://app_pg:app_pg_password@db/@0.0.0.0:5436/bwg_transactions?sslmode=disable' up
