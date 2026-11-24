# Order webhook retries

Status: draft

## Goal

When a partner's webhook endpoint fails, the order service retries the
delivery instead of dropping the event. Partners see each order event at
least once, in order per order id.

## Retry policy

A failed delivery is retried with **exponential backoff**: the first retry
waits 1s, and each later retry doubles the wait, capped at 30s. A delivery
is abandoned after 8 attempts and moves to the dead-letter queue.

A response counts as failed when the request times out after 10s, or the
endpoint answers with a 5xx status or 429. Any other 4xx status is final:
the event goes straight to the dead-letter queue.

## Ordering

Events for one order id are delivered one at a time. A later event waits
while an earlier one is being retried.

## Dead-letter queue

Abandoned events are kept for 14 days. Operators can replay them from the
admin page.

## Open questions

- Should a successful delivery reset the backoff for the partner?
- Is the 30s cap enough for partners with long maintenance windows?
