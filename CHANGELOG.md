# Changelog

## 1.1.0

- Standardize card typography and keep daily rules, reset time, and site instructions concise.

- Match dark-mode links, selected tabs, and calendar indicators to the gateway accent color.
- Remove the duplicate divider and padding around native pagination.
- Replace the redundant payout-limit explanation card with a compact settings link.
- Add localized configuration-field descriptions through the host form. Requires TokenRouter 0.6.1 or later.

- Redesign the check-in page with the gateway's native components and responsive layout.
- Add personal monthly calendars, paginated history, and exact completed-reward totals.
- Preserve recorded reward amounts across configuration changes and display small rewards without rounding them to zero.
- Add administrator history filters and a reward-settings overview with host configuration links.
- Allow new daily claims to be paused while retaining history and same-day pending recovery.
- Refresh at the host's next midnight and keep confirmed completion when the displayed balance cannot refresh.

## 1.0.0

- Add daily balance rewards with stable idempotency keys.
- Keep completed and pending claims in host KV across ordinary reinstalls.
- Add user check-in and administrator history pages using native host components.
