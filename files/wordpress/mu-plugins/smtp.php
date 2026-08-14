<?php
/**
 * Plugin Name: Homelab SMTP relay
 * Description: Route WordPress mail through the ocean exim smarthost hub (-> Brevo), so
 *   password resets / notifications actually send. The container has no sendmail; PHP
 *   mail() fails ("sendmail: not found") without this.
 *
 * mu-plugin so it auto-loads and survives image updates (wp-content is the persistent
 * volume). The hub does TLS to Brevo; the WP->hub hop is plaintext on the trusted LAN.
 */

add_action('phpmailer_init', function ($phpmailer) {
    // WP_SMTP_HOST is defined in wp-config via WORDPRESS_CONFIG_EXTRA (reliable under
    // Apache, unlike getenv); default to the docker gateway (= the ocean host).
    $host = defined('WP_SMTP_HOST') ? WP_SMTP_HOST : '172.26.0.1';
    $phpmailer->isSMTP();
    $phpmailer->Host       = $host;
    $phpmailer->Port       = 25;
    $phpmailer->SMTPAuth   = false;      // hub trusts the LAN relay_nets; no auth on this hop
    $phpmailer->SMTPSecure = '';         // no TLS to the hub (self-signed); hub->Brevo is TLS
    $phpmailer->SMTPAutoTLS = false;     // don't opportunistically STARTTLS to the self-signed hub
});

// From must be on the Brevo-authenticated domain (saetnere.com) so DKIM aligns.
add_filter('wp_mail_from', function ($from) {
    return (strpos($from, '@saetnere.com') !== false) ? $from : 'wordpress@saetnere.com';
});
add_filter('wp_mail_from_name', function ($name) {
    return $name ?: 'blog.saetnere.com';
});
