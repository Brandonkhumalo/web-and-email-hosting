<?php
// ============================================================
// Roundcube Webmail Configuration
// Variables substituted by user-data.sh at deploy time
// ============================================================

$config = [];

// Database
$config['db_dsnw'] = 'pgsql://roundcube:ROUNDCUBE_DB_PASSWORD_PLACEHOLDER@localhost/roundcubemail';

// IMAP — connect to local Dovecot
$config['imap_host'] = 'ssl://localhost:993';
$config['imap_conn_options'] = [
    'ssl' => ['verify_peer' => false, 'verify_peer_name' => false],
];

// SMTP — connect to local Postfix submission
$config['smtp_host'] = 'tls://localhost:587';
$config['smtp_user'] = '%u';
$config['smtp_pass'] = '%p';
$config['smtp_conn_options'] = [
    'ssl' => ['verify_peer' => false, 'verify_peer_name' => false],
];

// General
$config['product_name'] = 'Webmail';
$config['des_key'] = 'ROUNDCUBE_DES_KEY_PLACEHOLDER';
$config['plugins'] = ['archive', 'zipdownload'];
$config['skin'] = 'elastic';
$config['language'] = 'en_US';
$config['enable_installer'] = false;
$config['support_url'] = '';

// Security
$config['ip_check'] = true;
$config['session_lifetime'] = 30;
