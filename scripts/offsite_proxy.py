"""Send this process's urllib traffic through the offsite.sh SOCKS tunnel.

Importing it does nothing unless ALL_PROXY is a socks5/socks5h URL, which is
what `eval "$(scripts/offsite.sh env)"` sets off-LAN. urllib has no SOCKS
support and PySocks is not installed, so this is the SOCKS5 CONNECT handshake
by hand, installed as urllib's global opener. Hostnames go to the proxy
unresolved: off-LAN, only the far side can resolve *.home.
"""
import http.client
import os
import socket
import struct
import urllib.parse
import urllib.request

_proxy = None


def _recv(sock, n):
    buf = b""
    while len(buf) < n:
        chunk = sock.recv(n - len(buf))
        if not chunk:
            raise OSError("socks: proxy closed the connection")
        buf += chunk
    return buf


def _connect(host, port, timeout):
    sock = socket.create_connection(_proxy, timeout)
    try:
        sock.sendall(b"\x05\x01\x00")
        if _recv(sock, 2) != b"\x05\x00":
            raise OSError(f"socks: {_proxy[0]}:{_proxy[1]} refused no-auth")
        name = host.encode()
        sock.sendall(b"\x05\x01\x00\x03" + bytes([len(name)]) + name + struct.pack(">H", port))
        _, rep, _, atyp = _recv(sock, 4)
        if rep != 0:
            raise OSError(f"socks: connect to {host}:{port} failed (reply {rep})")
        _recv(sock, {1: 4, 4: 16}.get(atyp) or _recv(sock, 1)[0])
        _recv(sock, 2)
    except BaseException:
        sock.close()
        raise
    return sock


class _HTTPConnection(http.client.HTTPConnection):
    def connect(self):
        self.sock = _connect(self.host, self.port, self.timeout)


class _HTTPSConnection(http.client.HTTPSConnection):
    def connect(self):
        self.sock = self._context.wrap_socket(_connect(self.host, self.port, self.timeout),
                                              server_hostname=self.host)


class _HTTPHandler(urllib.request.HTTPHandler):
    def http_open(self, req):
        return self.do_open(_HTTPConnection, req)


class _HTTPSHandler(urllib.request.HTTPSHandler):
    def https_open(self, req):
        return self.do_open(_HTTPSConnection, req, context=self._context)


_url = urllib.parse.urlsplit(os.environ.get("ALL_PROXY") or os.environ.get("all_proxy") or "")
if _url.scheme in ("socks5", "socks5h"):
    _proxy = (_url.hostname, _url.port or 1080)
    urllib.request.install_opener(urllib.request.build_opener(_HTTPHandler, _HTTPSHandler))
