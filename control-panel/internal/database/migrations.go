package database

import (
	"context"
	"fmt"
	"log"
)

// RunMigrations creates all tables if they don't exist.
// Uses IF NOT EXISTS so it's safe to run on every startup.
func (db *DB) RunMigrations(ctx context.Context) error {
	log.Println("Running database migrations...")

	migrations := []string{
		// ============================================================
		// Core tables
		// ============================================================

		// Plans table (billing tiers for customers)
		`CREATE TABLE IF NOT EXISTS plans (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(100) NOT NULL UNIQUE,
			max_sites INTEGER DEFAULT 3,
			max_email_accounts INTEGER DEFAULT 10,
			max_storage_mb INTEGER DEFAULT 1024,
			price_cents INTEGER DEFAULT 0,
			active BOOLEAN DEFAULT TRUE
		)`,

		// Customers table
		`CREATE TABLE IF NOT EXISTS customers (
			id BIGSERIAL PRIMARY KEY,
			email VARCHAR(255) NOT NULL UNIQUE,
			password VARCHAR(255) NOT NULL,
			name VARCHAR(255) NOT NULL,
			company VARCHAR(255) DEFAULT '',
			plan_id BIGINT REFERENCES plans(id) DEFAULT NULL,
			active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)`,

		// Domains table
		`CREATE TABLE IF NOT EXISTS domains (
			id BIGSERIAL PRIMARY KEY,
			customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
			name VARCHAR(255) NOT NULL UNIQUE,
			route53_zone_id VARCHAR(255) DEFAULT '',
			ns_verified BOOLEAN DEFAULT FALSE,
			ssl_status VARCHAR(50) DEFAULT 'pending',
			cert_path VARCHAR(512) DEFAULT '',
			cert_key_path VARCHAR(512) DEFAULT '',
			email_enabled BOOLEAN DEFAULT FALSE,
			ses_verified BOOLEAN DEFAULT FALSE,
			active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)`,

		// Sites table (static sites served by Nginx, backend sites as Docker containers)
		`CREATE TABLE IF NOT EXISTS sites (
			id BIGSERIAL PRIMARY KEY,
			customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
			domain_id BIGINT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
			type VARCHAR(20) NOT NULL CHECK (type IN ('static', 'backend')),
			subdomain VARCHAR(255) DEFAULT '@',
			active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),

			-- Nginx config
			nginx_config_path VARCHAR(512) DEFAULT '',

			-- Static site fields
			site_root VARCHAR(512) DEFAULT '',

			-- Backend site fields (Docker container)
			container_name VARCHAR(255) DEFAULT '',
			host_port INTEGER DEFAULT 0,
			container_port INTEGER DEFAULT 8080,
			docker_image VARCHAR(512) DEFAULT ''
		)`,

		// Domain nameservers (stored separately since it's an array)
		`CREATE TABLE IF NOT EXISTS domain_nameservers (
			id BIGSERIAL PRIMARY KEY,
			domain_id BIGINT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
			nameserver VARCHAR(255) NOT NULL
		)`,

		// ============================================================
		// Email tables (full mailbox hosting via Postfix + Dovecot)
		// ============================================================

		// Email accounts (real mailboxes — IMAP login, SMTP sending)
		`CREATE TABLE IF NOT EXISTS email_accounts (
			id BIGSERIAL PRIMARY KEY,
			domain_id BIGINT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
			email VARCHAR(255) NOT NULL UNIQUE,
			display_name VARCHAR(255) DEFAULT '',
			password VARCHAR(255) DEFAULT '',
			maildir VARCHAR(512) DEFAULT '',
			quota BIGINT DEFAULT 1073741824,
			mail_enabled BOOLEAN DEFAULT FALSE,
			ses_verified BOOLEAN DEFAULT FALSE,
			active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)`,

		// Add columns if table already exists (safe for upgrades)
		`ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS password VARCHAR(255) DEFAULT ''`,
		`ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS maildir VARCHAR(512) DEFAULT ''`,
		`ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS quota BIGINT DEFAULT 1073741824`,
		`ALTER TABLE email_accounts ADD COLUMN IF NOT EXISTS mail_enabled BOOLEAN DEFAULT FALSE`,

		// Email aliases (forwarding rules)
		`CREATE TABLE IF NOT EXISTS email_aliases (
			id BIGSERIAL PRIMARY KEY,
			domain_id BIGINT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
			source VARCHAR(255) NOT NULL,
			destination VARCHAR(255) NOT NULL,
			active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT NOW()
		)`,

		// ============================================================
		// Indexes
		// ============================================================

		`CREATE INDEX IF NOT EXISTS idx_domains_customer_id ON domains(customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_domains_name ON domains(name)`,
		`CREATE INDEX IF NOT EXISTS idx_sites_customer_id ON sites(customer_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sites_domain_id ON sites(domain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_domain_nameservers_domain_id ON domain_nameservers(domain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_accounts_domain_id ON email_accounts(domain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_accounts_email ON email_accounts(email)`,
		`CREATE INDEX IF NOT EXISTS idx_email_aliases_domain_id ON email_aliases(domain_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_aliases_source ON email_aliases(source)`,

		// ============================================================
		// Triggers
		// ============================================================

		`CREATE OR REPLACE FUNCTION update_updated_at_column()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = NOW();
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql`,

		`DO $$ BEGIN
			CREATE TRIGGER trg_customers_updated_at
				BEFORE UPDATE ON customers
				FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
		EXCEPTION WHEN duplicate_object THEN NULL;
		END $$`,

		`DO $$ BEGIN
			CREATE TRIGGER trg_domains_updated_at
				BEFORE UPDATE ON domains
				FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
		EXCEPTION WHEN duplicate_object THEN NULL;
		END $$`,

		`DO $$ BEGIN
			CREATE TRIGGER trg_sites_updated_at
				BEFORE UPDATE ON sites
				FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
		EXCEPTION WHEN duplicate_object THEN NULL;
		END $$`,

		`DO $$ BEGIN
			CREATE TRIGGER trg_email_accounts_updated_at
				BEFORE UPDATE ON email_accounts
				FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
		EXCEPTION WHEN duplicate_object THEN NULL;
		END $$`,

		// ============================================================
		// Default plan
		// ============================================================

		`INSERT INTO plans (name, max_sites, max_email_accounts, max_storage_mb, price_cents)
		VALUES ('starter', 3, 5, 1024, 0)
		ON CONFLICT (name) DO NOTHING`,
	}

	for i, sql := range migrations {
		if _, err := db.Pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	log.Printf("Migrations complete (%d statements)\n", len(migrations))
	return nil
}
