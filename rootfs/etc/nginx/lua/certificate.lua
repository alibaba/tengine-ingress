local ssl = require("ngx.ssl")
local re_sub = ngx.re.sub

local _M = {}

local DEFAULT_CERT_HOSTNAME = "_"

local certificate_data = ngx.shared.certificate_data
local certificate_servers = ngx.shared.certificate_servers

local function get_der_cert_and_priv_key(pem_cert_key)
  local der_cert, der_cert_err = ssl.cert_pem_to_der(pem_cert_key)
  if not der_cert then
    return nil, nil, "failed to convert certificate chain from PEM to DER: " .. der_cert_err
  end

  local der_priv_key, dev_priv_key_err = ssl.priv_key_pem_to_der(pem_cert_key)
  if not der_priv_key then
    return nil, nil, "failed to convert private key from PEM to DER: " .. dev_priv_key_err
  end

  return der_cert, der_priv_key, nil
end

local function set_der_cert_and_key(der_cert, der_priv_key)
  local set_cert_ok, set_cert_err = ssl.set_der_cert(der_cert)
  if not set_cert_ok then
    return "failed to set DER cert: " .. set_cert_err
  end

  local set_priv_key_ok, set_priv_key_err = ssl.set_der_priv_key(der_priv_key)
  if not set_priv_key_ok then
    return "failed to set DER private key: " .. set_priv_key_err
  end
end

local function split_str(s, delimiter)
  local result = {};
  for match in (s..delimiter):gmatch("(.-)"..delimiter) do
      table.insert(result, match);
  end
  return result;
end

local function get_cert_uids(hostname)
  local uids = {}
  local uidstr = certificate_servers:get(hostname)
  if uidstr then
    uids = split_str(uidstr, "#")
  end

  return uids
end

local function get_pem_cert_uid(raw_hostname)
  local hostname = re_sub(raw_hostname, "\\.$", "", "jo")

  local uids = get_cert_uids(hostname)
  if next(uids) then
    return uids
  end

  local wildcard_hosatname, _, err = re_sub(hostname, "^[^\\.]+\\.", "*.", "jo")
  if err then
    ngx.log(ngx.ERR, "error: ", err)
    return uids
  end

  if wildcard_hosatname then
    uids = get_cert_uids(wildcard_hosatname)
  end

  return uids
end

function _M.configured_for_current_request()
  if ngx.ctx.configured_for_current_request ~= nil then
    return ngx.ctx.configured_for_current_request
  end

  local pem_cert_uids = get_pem_cert_uid(ngx.var.host)  
  ngx.ctx.configured_for_current_request = next(pem_cert_uids) ~= nil

  return ngx.ctx.configured_for_current_request
end

-- multiple certificates support.
function _M.call()
  local hostname, hostname_err = ssl.server_name()
  if hostname_err then
    ngx.log(ngx.ERR, "error while obtaining hostname: " .. hostname_err)
  end
  if not hostname then
    ngx.log(ngx.INFO,
      "obtained hostname is nil (the client does not support SNI?), falling back to default certificate")
    hostname = DEFAULT_CERT_HOSTNAME
  end

  local pem_cert
  local pem_cert_uids = get_pem_cert_uid(hostname)
  if next(pem_cert_uids) == nil then
    pem_cert_uids = get_pem_cert_uid(DEFAULT_CERT_HOSTNAME)
  end
  if next(pem_cert_uids) ~= nil then
    local clear_ok, clear_err = ssl.clear_certs()
    if not clear_ok then
      ngx.log(ngx.ERR, "failed to clear existing (fallback) certificates: " .. clear_err)
      return ngx.exit(ngx.ERROR)
    end

    for i = 1, #pem_cert_uids do
      pem_cert = certificate_data:get(pem_cert_uids[i])
      if not pem_cert then
        ngx.log(ngx.ERR, string.format("certificate not found for uid %s of hostname %s", pem_cert_uids[i], tostring(hostname)))
        goto continue
      end

      local der_cert, der_priv_key, der_err = get_der_cert_and_priv_key(pem_cert)
      if der_err then
        ngx.log(ngx.ERR, der_err)
        goto continue
      end

      local set_der_err = set_der_cert_and_key(der_cert, der_priv_key)
      if set_der_err then
        ngx.log(ngx.ERR, set_der_err)
        goto continue
      end
      ::continue::
    end
  end

  -- TODO: based on `der_cert` find OCSP responder URL
  -- make OCSP request and POST it there and get the response and staple it to
  -- the current SSL connection if OCSP stapling is enabled
end

return _M
