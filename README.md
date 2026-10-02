🛒 FlashCart — Scalable Marketplace & Flash-Sale Platform

A production-inspired Flipkart/Amazon-style e-commerce platform built to demonstrate high-concurrency flash sales, distributed services, secure payments, AI-assisted shopping, automated festival campaigns, and real-time observability.

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.27-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/React-Vite-61DAFB?style=for-the-badge&logo=react&logoColor=black" alt="React">
  <img src="https://img.shields.io/badge/PostgreSQL-16-336791?style=for-the-badge&logo=postgresql&logoColor=white" alt="PostgreSQL">
  <img src="https://img.shields.io/badge/Redis-7-DC382D?style=for-the-badge&logo=redis&logoColor=white" alt="Redis">
  <img src="https://img.shields.io/badge/Kafka-Redpanda-231F20?style=for-the-badge&logo=apachekafka&logoColor=white" alt="Kafka">
  <img src="https://img.shields.io/badge/Razorpay-Test_Mode-0C2451?style=for-the-badge" alt="Razorpay">
</p>

<p align="center">
  <b>FlashCart is not just a CRUD e-commerce application.</b><br>
  It is designed as a distributed-system learning and portfolio project around the problems that appear when thousands of users compete for limited inventory at the same time.
</p>

✨ Why FlashCart?

A normal shopping application can be built with a frontend, backend, and database.

A flash-sale platform is much harder.

Imagine a product has only 10 units available while thousands of users try to buy it simultaneously. The system has to prevent overselling, keep checkout responsive, handle duplicate requests, process payments safely, recover from failures, and still provide useful monitoring.

FlashCart was built around these engineering problems.

🎯 Core engineering goals

⚡ Handle high-concurrency product and flash-sale traffic

🔒 Prevent overselling using atomic inventory operations

♻️ Support idempotent operations for retry-safe requests

📨 Decouple asynchronous work using Kafka/Redpanda

💳 Integrate Razorpay Test Mode with server-side verification

🤖 Provide an AI shopping assistant grounded in catalog data

📊 Expose application metrics through Prometheus

📈 Visualize operational metrics with Grafana

🧾 Generate immutable PDF invoices

📧 Process notification/email jobs asynchronously

🎉 Automate festival campaigns and flash-sale events

🛡️ Keep secrets and payment verification on the server

🧪 Support automated backend/frontend testing and load testing

🚀 Feature Overview

🛍️ Customer Storefront

FlashCart provides a complete marketplace-style shopping experience.

Homepage

Marketplace-style header and navigation

Search

Category navigation

Promotional hero section

Dynamic announcement strip

Flash-deal section

Product recommendations

Festival/seasonal campaigns

Responsive mobile layout

Product Discovery

Search products

Filter by category

Filter by brand

Filter by price

Sort products

View product details

View offers and delivery information

Product ratings/reviews presentation

Wishlist

Product sharing

Shopping

Add/remove products from cart

Update quantities

Wishlist management

Checkout

Coupon application

Saved delivery addresses

Order history

Order tracking

⚡ Flash Sale Engine

The flash-sale system is one of the main distributed-system components of FlashCart.

The challenge:

             Thousands of Users
                    │
        ┌───────────┼───────────┐
        ▼           ▼           ▼
      User A      User B      User C
        │           │           │
        └───────────┼───────────┘
                    ▼
              Flash Sale API
                    │
                    ▼
              Redis Inventory
                    │
          Atomic stock operation
                    │
          ┌─────────┴─────────┐
          ▼                   ▼
      SUCCESS              SOLD OUT
          │
          ▼
       Checkout

FlashCart uses Redis for fast flash-sale inventory operations rather than forcing every high-frequency inventory request directly through PostgreSQL.

Flash-sale capabilities

Temporary flash-sale prices

Redis-backed inventory

Atomic stock operations

High-speed inventory checks

Sold-out handling

Flash-sale analytics

Admin flash-sale management

Festival-triggered flash sales

Unsold-stock restoration

The base product price is not permanently overwritten by a temporary festival/flash-sale price.

🏗️ System Architecture

                         ┌──────────────────────┐
                         │      React/Vite      │
                         │     Storefront       │
                         └──────────┬───────────┘
                                    │
                              HTTP / REST
                                    │
                                    ▼
                         ┌──────────────────────┐
                         │       Go API         │
                         │      Chi Router      │
                         └──────────┬───────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              │                     │                     │
              ▼                     ▼                     ▼
       ┌─────────────┐       ┌─────────────┐       ┌─────────────┐
       │ PostgreSQL  │       │    Redis    │       │   Razorpay  │
       │             │       │             │       │ Test Mode   │
       │ users       │       │ cart        │       │             │
       │ products    │       │ inventory   │       │ payments    │
       │ orders      │       │ idempotency │       │ checkout    │
       │ payments    │       │ sale price  │       │ webhooks    │
       │ coupons     │       └─────────────┘       └─────────────┘
       │ addresses   │
       └─────────────┘
              │
              │ events / jobs
              ▼
       ┌──────────────────┐
       │ Kafka / Redpanda │
       └────────┬─────────┘
                │
        ┌───────┴──────────┐
        ▼                  ▼
┌─────────────────┐ ┌────────────────────┐
│ Order Worker    │ │ Notification       │
│                 │ │ Worker             │
│ async order     │ │ email / invoice    │
│ processing      │ │ retries            │
└─────────────────┘ └────────────────────┘

                Observability
                     │
          ┌──────────┴──────────┐
          ▼                     ▼
    ┌────────────┐       ┌────────────┐
    │ Prometheus │──────▶│  Grafana   │
    └────────────┘       └────────────┘

🔥 Complete Order & Payment Flow

Customer
   │
   ▼
Add Product
   │
   ▼
Cart
   │
   ▼
Checkout
   │
   ├──────────────► Apply Coupon
   │
   ▼
Create FlashCart Order
PAYMENT_PENDING
   │
   ▼
Create Razorpay Order
   │
   ▼
Razorpay Checkout
   │
   ▼
Payment Completed
   │
   ▼
payment_id + signature
   │
   ▼
Go API
   │
   ▼
Server-side HMAC verification
   │
   ├──── Invalid ────► Reject / keep unpaid
   │
   ▼
PAID
   │
   ▼
CONFIRMED
   │
   ├──────────────► Generate PDF Invoice
   │
   ├──────────────► Write notification job
   │
   └──────────────► Publish asynchronous events

Payment state transition

PAYMENT_PENDING
       │
       ▼
   Razorpay
       │
       ▼
Signature Verification
       │
       ▼
      PAID
       │
       ▼
   CONFIRMED

The browser is never trusted as the source of payment truth.

💳 Razorpay Integration

FlashCart integrates Razorpay Test Mode for payment processing.

Implemented

Razorpay Test Mode checkout

Server-side Razorpay order creation

HMAC signature verification

PostgreSQL payment records

Razorpay webhook endpoint

Admin payment monitoring

COD support for demo orders

UPI/QR, cards, net banking and wallets through Checkout configuration

Security model

React
  │
  │ public Key ID only
  ▼
Razorpay Checkout
  │
  │ payment_id
  │ order_id
  │ signature
  ▼
Go API
  │
  │ secret key stays here
  ▼
HMAC Verification
  │
  ├── ❌ Invalid → reject
  │
  └── ✅ Valid → mark payment paid

Important

RAZORPAY_KEY_SECRET must never be placed in React/Vite environment variables.

Use Razorpay Test Mode for development and demonstration. No real customer money should be used.

🧾 Invoice & Notification System

After a successful order:

Confirmed Order
      │
      ├──────────────► PDF Invoice
      │
      └──────────────► PostgreSQL Outbox
                              │
                              ▼
                    Notification Worker
                              │
                       SMTP / Email
                              │
                       Retry on failure

Invoice capabilities

Immutable numbered PDF invoices

Generated from server-side order/customer data

Download invoice

Print invoice

Private order-owner authorization

Non-cacheable invoice responses

Tax-inclusive product pricing model

Reliable notification processing

The notification worker:

Polls the PostgreSQL outbox

Leases jobs safely across replicas

Retries failed SMTP operations

Uses capped exponential backoff

Supports recovery scans

Sends invoices as email attachments when SMTP is configured

If SMTP is not configured, invoices are still generated and remain available for download/printing.

🎟️ Coupon Engine

FlashCart contains a server-side coupon validation system.

Demo coupons

Examples include:

FLASH10
SBI10
HDFC12
ICICI8
AXIS10
PHONEPE50
PAYTM30
FESTIVE10
NEWUSER15
GROCERY5
SNACKS10
SAVE50
SAVE100

Coupon rules

The checkout service validates:

Cart subtotal

Maximum usage

Category subtotal

First-order eligibility

Payment method

Per-user redemption history

Coupon reservation

Current product prices

Only one coupon is accepted per order.

For bank/wallet-specific coupons, the payment instrument/issuer must match the configured offer.

🤖 AI Shopping Assistant

FlashCart includes an AI-powered shopping assistant for catalog-grounded shopping help.

How it works

User Question
      │
      ▼
Current Search / Category / Product Context
      │
      ▼
Matching Catalog Records
      │
      ▼
AI Context
      │
      ▼
Groq LLM
      │
      ▼
Shopping Recommendation / Comparison

The assistant can use:

Current search context

Active category

Brand filters

Current product page

Up to eight relevant catalog records

AI safety behavior

The assistant does not invent customer-review summaries.

If verified review data is unavailable, it explicitly tells the user that review-based information is unavailable.

Configuration

Set the following in the project-root .env:

GROQ_API_KEY=your_key_here
GROQ_MODEL=openai/gpt-oss-120b

The API key is read only by the Go backend.

Do not expose it through a VITE_ variable.

Requests are limited to eight assistant requests per client IP per minute.

🎉 Automated Festival Campaigns

FlashCart includes a festivalworker for automated seasonal campaigns.

Calendar Provider
       │
       ▼
Festival Calendar Sync
       │
       ▼
PostgreSQL
       │
       ├──────────────► 7-day reminder
       │
       ├──────────────► Festival announcement
       │
       └──────────────► Flash sale at local midnight
                              │
                              ▼
                           Redis
                              │
                              ▼
                       Temporary prices
                              │
                              ▼
                     Restore unsold stock

Supported workflow

Sync public holidays

Store national/regional events

Seven-day reminders

Festival announcements

Automatic flash-sale scheduling

Temporary Redis sale pricing

Unsold-stock restoration

Prometheus metrics

India configuration

Default timezone:

FESTIVAL_TIMEZONE=Asia/Kolkata

Calendarific is the default provider for India.

Example configuration:

HOLIDAY_COUNTRY_CODE=IN
CALENDARIFIC_API_KEY=your_key_here
FESTIVAL_DISCOUNT_PERCENT=10
FESTIVAL_STOCK_PER_PRODUCT=10
FESTIVAL_PRODUCTS_PER_EVENT=20

Browser push uses VAPID credentials.

Generate them with:

go run ./cmd/vapidkeys

Keep the private key secret.

🧑‍💼 Admin Dashboard

FlashCart includes an administrative control layer.

Admin capabilities

📊 Dashboard

💰 Sales overview

👥 User management

📦 Product CRUD

📈 Revenue monitoring

🏷️ Inventory management

⚡ Flash-sale management

📢 Announcement/broadcast queue

📊 Flash-sale analytics

🧾 Recent order operations

💳 Payment monitoring

🎟️ Coupon usage analytics

Development credentials

Email:    admin@flashcart.local
Password: Admin@12345

Change these credentials before any non-demo deployment.

📊 Observability

FlashCart includes a monitoring stack based on Prometheus + Grafana.

             FlashCart API
                  │
                  │ metrics
                  ▼
             Prometheus
                  │
                  ▼
               Grafana

Services

Service

Purpose

Default URL

FlashCart API

REST backend

http://localhost:8080

API Health

Health check

http://localhost:8080/healthz

Prometheus

Metrics

http://localhost:9091

Grafana

Dashboards

http://localhost:3000

Frontend

React storefront

http://localhost:5173

🧪 Load Testing

FlashCart was also tested with k6 to understand API behavior under concurrent traffic.

Example load-test command:

k6 run loadtest.js

The project has been exercised with increasing virtual-user loads, including a 100-VU test scenario.

A representative 100-VU run achieved:

Checks:        611,650
Checks/sec:    ~20,383
Success rate:  100%
Duration:      30 seconds

These figures are benchmark results from the development environment, not a claim that the production system can sustain the same traffic without infrastructure-specific capacity testing.

The goal of the load testing is to demonstrate the engineering approach to concurrency, caching, database access, and API scalability.

🧩 Distributed Systems Components

FlashCart uses multiple infrastructure components, each with a specific responsibility.

Component

Responsibility

Go

High-performance backend API

React + Vite

Interactive marketplace frontend

PostgreSQL

Durable transactional data

Redis

Cache, cart, inventory, idempotency, temporary sale pricing

Kafka / Redpanda

Asynchronous event processing

Order Worker

Background order processing

Notification Worker

Invoice/email processing

Festival Worker

Automated seasonal campaigns

Razorpay

Payment checkout and verification

Prometheus

Metrics collection

Grafana

Metrics visualization

Groq

AI shopping assistant

Docker Compose

Local multi-service orchestration

k6

Load testing

🛡️ Reliability & Security Design

FlashCart includes several production-inspired safeguards.

Payment security

Server-side payment verification

HMAC signature verification

Secret keys never exposed to frontend

Webhook support

Payment state transitions

Inventory protection

Redis-backed flash-sale inventory

Atomic inventory operations

Temporary sale pricing

Inventory restoration after festival campaigns

Idempotency

Redis is used for idempotency-related operations so retrying requests does not blindly create duplicate business operations.

Authorization

Private invoice endpoints require the authenticated order owner.

Secrets

Keep credentials in .env and never commit:

RAZORPAY_KEY_SECRET
RAZORPAY_WEBHOOK_SECRET
GROQ_API_KEY
SMTP_PASSWORD
CALENDARIFIC_API_KEY
VAPID_PRIVATE_KEY

🐳 Docker Architecture

The project can be started as a multi-service environment with Docker Compose.

Typical services include:

flashcart-api
flashcart-postgres
flashcart-redis
flashcart-kafka
flashcart-orderworker
flashcart-notificationworker
flashcart-festivalworker
flashcart-prometheus
flashcart-grafana

Start everything:

docker compose up -d --build

Check running containers:

docker compose ps

View API logs:

docker compose logs -f api

⚙️ Installation & Local Setup

1. Clone the repository

git clone <YOUR_REPOSITORY_URL>
cd FlashCart

2. Configure environment variables

Copy:

Copy-Item .env.example .env

Configure at minimum:

RAZORPAY_KEY_ID=your_test_key
RAZORPAY_KEY_SECRET=your_test_secret
RAZORPAY_WEBHOOK_SECRET=your_webhook_secret

For AI:

GROQ_API_KEY=your_groq_key
GROQ_MODEL=openai/gpt-oss-120b

For email/invoices:

SMTP_HOST=your_smtp_host
SMTP_PORT=587
SMTP_USERNAME=your_username
SMTP_PASSWORD=your_password
SMTP_FROM=your_sender

3. Start backend infrastructure

docker compose up -d --build

4. Start frontend

cd frontend
npm install
npm run dev

Open:

http://localhost:5173

💳 Test Razorpay Payment

Register/login as a normal FlashCart user.

Add a product to the cart.

Open checkout.

Select Razorpay.

Apply FLASH10 when the cart subtotal is at least ₹500.

Click Pay.

Razorpay Checkout opens.

Use Razorpay's official Test Mode credentials.

FlashCart sends the Razorpay order/payment/signature information to the Go API.

The server verifies the signature.

The payment transitions from:

PAYMENT_PENDING
        ↓
      PAID
        ↓
   CONFIRMED

The old simulated order worker does not auto-charge Razorpay orders.

🧪 Build & Test

Backend

go mod tidy
go test ./...
go vet ./...

Frontend

cd frontend
npm install
npm run build

Load testing

k6 run loadtest.js

📁 High-Level Project Structure

FlashCart/
│
├── cmd/
│   ├── api/
│   ├── orderworker/
│   ├── notificationworker/
│   ├── festivalworker/
│   └── vapidkeys/
│
├── internal/
│   ├── handlers/
│   ├── services/
│   ├── repositories/
│   ├── middleware/
│   ├── workers/
│   ├── notify/
│   └── ...
│
├── frontend/
│   ├── src/
│   ├── public/
│   ├── package.json
│   └── vite.config.*
│
├── docs/
│   └── ENTERPRISE_ADMIN_BLUEPRINT.md
│
├── docker-compose.yml
├── .env.example
├── go.mod
├── go.sum
└── loadtest.js

The exact package/file layout can evolve as the project grows. The enterprise admin blueprint documents the target service boundaries and production-oriented rollout plan.

🔄 End-to-End User Journey

                    ┌───────────────┐
                    │     User      │
                    └───────┬───────┘
                            │
                            ▼
                    Browse Products
                            │
                            ▼
                      Search / Filter
                            │
                            ▼
                       Product Page
                            │
                 ┌──────────┴──────────┐
                 │                     │
                 ▼                     ▼
              Wishlist               Cart
                                       │
                                       ▼
                                   Checkout
                                       │
                           ┌───────────┴───────────┐
                           │                       │
                           ▼                       ▼
                        Coupon                 Payment
                           │                       │
                           └───────────┬───────────┘
                                       ▼
                                  Order Created
                                       │
                                       ▼
                                Payment Verified
                                       │
                                       ▼
                                   Confirmed
                                       │
                     ┌─────────────────┼─────────────────┐
                     ▼                 ▼                 ▼
                  Invoice          Order Event       Notification
                     │                 │                 │
                     ▼                 ▼                 ▼
                   PDF             Worker           Email/SMTP

🔮 Production Roadmap

The project intentionally distinguishes between implemented demo capabilities and future production integrations.

Potential next steps:

Kubernetes deployment

Horizontal API scaling

Managed PostgreSQL

Managed Redis

Production Kafka cluster

Distributed tracing with OpenTelemetry

Centralized logging

API gateway / rate limiting at edge

CDN for static assets

Object storage for product images

Production Razorpay webhook hardening

Real OTP provider

Google/Apple authentication

WhatsApp Business messaging

Production SMS provider

Automated refund workflows

Advanced recommendation model

Kubernetes autoscaling

Chaos/failure testing

⚠️ Provider-Dependent Features

Some workflows require external provider credentials/configuration.

These include:

OTP login

Google/Apple/social authentication

Real email delivery

WhatsApp Business messaging

Live bank offers/cashback

Production refund automation

Festival calendar API access

SMS delivery

Browser push notifications

The demo does not claim fake provider credentials or simulated external transactions are real integrations.

🔐 Security Checklist

Before deploying outside a local/demo environment:

Change default admin credentials

Configure production secrets securely

Never commit .env

Never expose Razorpay secret keys

Configure and verify Razorpay webhooks

Configure HTTPS/TLS

Restrict CORS

Add production rate limiting

Rotate credentials periodically

Use managed secret storage

Review database permissions

Configure production SMTP securely

Enable centralized logs and monitoring

📈 What This Project Demonstrates

FlashCart is designed to demonstrate more than frontend/backend CRUD development.

Backend Engineering

REST API development with Go

Middleware and request validation

PostgreSQL data modeling

Redis-based high-speed operations

Background workers

Event-driven architecture

Distributed Systems

Asynchronous processing

Kafka/Redpanda messaging

Idempotency

Atomic inventory operations

Worker-based architecture

Outbox pattern

Retry and recovery workflows

System Design

Separation of synchronous and asynchronous workloads

Durable vs temporary data

Payment state machines

Flash-sale concurrency

Failure recovery

Observability

DevOps & Reliability

Docker Compose

Prometheus

Grafana

Health checks

Load testing

Service-level monitoring

Modern Application Features

AI shopping assistant

Payment integration

Automated festival campaigns

PDF invoices

Email notifications

Coupon engine

Admin analytics

📚 Architecture Documentation

For deeper architecture decisions, service boundaries, workflows, security controls, deployment guidance, and verification gates, see:

docs/ENTERPRISE_ADMIN_BLUEPRINT.md

This document separates the current implementation from planned production capabilities.

🧠 Key Engineering Lessons

Building FlashCart highlights several practical backend lessons:

Redis is useful for extremely hot, temporary operations—but PostgreSQL remains the durable source of truth for business data.

A payment success message from the browser is not enough. Payment authenticity must be verified server-side.

Background workers prevent slow operations such as notifications and invoice delivery from blocking the customer-facing request path.

Flash sales are fundamentally concurrency problems, not just UI features.

Observability is part of system design, not an afterthought.

🏆 Project Highlights

⚡ High-concurrency Flash Sales
🛒 Full Marketplace Experience
💳 Razorpay Test Payments
🤖 AI Shopping Assistant
📦 Redis Inventory Management
📨 Kafka / Redpanda Events
🧾 PDF Invoice Generation
📧 Async Notification Worker
🎉 Automated Festival Campaigns
📊 Prometheus + Grafana
🐳 Dockerized Infrastructure
🧪 k6 Load Testing
🔐 Server-side Security Controls

👨‍💻 Built With

Backend: Go, Chi Router, PostgreSQL, Redis, Kafka/Redpanda
Frontend: React, Vite
Payments: Razorpay Test Mode
AI: Groq
Messaging: Kafka / Redpanda
Monitoring: Prometheus, Grafana
Testing: Go Test, Go Vet, k6
Infrastructure: Docker, Docker Compose

📌 Development Note

FlashCart is a portfolio/demo project with production-inspired architecture. Some integrations depend on third-party provider credentials and local infrastructure configuration.

The architecture is intentionally designed to demonstrate how an e-commerce system can evolve from a simple marketplace into a distributed, observable, event-driven platform.
