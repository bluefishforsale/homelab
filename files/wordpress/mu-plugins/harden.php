<?php
/**
 * Plugin Name: Homelab XML-RPC hardening
 * Description: Neuters the XML-RPC attack surface (auth brute-force amplification + pingback reflection).
 *
 * Lives in mu-plugins so it auto-loads at the right point in the request (add_filter is
 * not available yet in wp-config.php / WORDPRESS_CONFIG_EXTRA) and survives image updates
 * because wp-content is the persistent host volume.
 */

// Disable XML-RPC methods that require authentication -> kills system.multicall brute-force amplification.
add_filter('xmlrpc_enabled', '__return_false');

// Strip pingback methods (no auth required, used for reflective DDoS and enumeration).
add_filter('xmlrpc_methods', function ($methods) {
    unset($methods['pingback.ping'], $methods['pingback.extensions.getPingbacks']);
    return $methods;
});

// Stop advertising the endpoint.
add_filter('wp_headers', function ($headers) {
    unset($headers['X-Pingback']);
    return $headers;
});
