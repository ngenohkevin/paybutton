-- Rollback migration - drop all address pool tables

DROP INDEX IF EXISTS idx_queue_site_order;
DROP INDEX IF EXISTS idx_addresses_status;
DROP INDEX IF EXISTS idx_addresses_reserved;
DROP INDEX IF EXISTS idx_addresses_email;
DROP INDEX IF EXISTS idx_addresses_address;
DROP INDEX IF EXISTS idx_addresses_site_index;
DROP INDEX IF EXISTS idx_addresses_site_status;
DROP INDEX IF EXISTS idx_pool_state_updated;

DROP TABLE IF EXISTS address_pool_queue;
DROP TABLE IF EXISTS address_pool_addresses;
DROP TABLE IF EXISTS address_pool_state;
