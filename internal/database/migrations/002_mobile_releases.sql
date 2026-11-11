CREATE TABLE mobile_app_releases (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 version_name VARCHAR(80) NOT NULL,
 version_code BIGINT NOT NULL UNIQUE,
 package_name VARCHAR(200) NOT NULL,
 apk_path VARCHAR(100) NOT NULL,
 sha256 VARCHAR(64) NOT NULL,
 signer_sha256 VARCHAR(64) NOT NULL,
 file_size BIGINT NOT NULL,
 changelog TEXT NOT NULL,
 force_update BOOLEAN NOT NULL DEFAULT FALSE,
 status VARCHAR(20) NOT NULL DEFAULT 'draft',
 published_at DATETIME(6) NULL,
 created_by BIGINT UNSIGNED NOT NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 INDEX mobile_release_status (status)
);
CREATE TABLE mobile_release_policies (
 id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
 current_release_id BIGINT UNSIGNED NULL,
 minimum_supported_version_code BIGINT NOT NULL DEFAULT 0,
 highest_version_code BIGINT NOT NULL DEFAULT 0
);
INSERT INTO mobile_release_policies (id) VALUES (1);
CREATE TABLE mobile_release_audits (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 release_id BIGINT UNSIGNED NOT NULL,
 actor_id BIGINT UNSIGNED NOT NULL,
 action VARCHAR(80) NOT NULL,
 details TEXT NOT NULL,
 created_at DATETIME(6) NOT NULL
);
