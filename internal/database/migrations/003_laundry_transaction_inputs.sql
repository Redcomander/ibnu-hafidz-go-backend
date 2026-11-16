CREATE TABLE laundry_transaction_inputs (
 transaction_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
 actor_id BIGINT UNSIGNED NOT NULL,
 actor_name VARCHAR(255) NOT NULL,
 created_at DATETIME(6) NOT NULL
);
