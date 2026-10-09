---
id: 2
type: story
title: "Shoppers can apply a discount code at checkout"
parent: epic-cart-rules
covers: [FR-4]
after: [1]
assignee: ""
refined: true
hitl: false
risk: high
---

# Shoppers can apply a discount code at checkout

## Description

A shopper enters a code at checkout and sees the discounted total before paying.

## Acceptance Criteria

1. **Valid code reduces the total**
   **Given** a cart of 200000 and an active 10% code
   **When** the shopper applies the code
   **Then** the total shows 180000
2. **Expired code is refused**
   **Given** an expired code
   **When** the shopper applies it
   **Then** the total is unchanged and the reason is shown

## Boundaries

- Must not change: tax calculation
