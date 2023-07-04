local socket = ngx.socket.tcp
local cjson = require("cjson.safe")
local assert = assert
local new_tab = require "table.new"
local clear_tab = require "table.clear"
local clone_tab = require "table.clone"
local nkeys = require "table.nkeys"

-- if an Nginx worker processes more than (MAX_BATCH_SIZE/FLUSH_INTERVAL) RPS then it will start dropping metrics
local MAX_BATCH_SIZE = 10000
local FLUSH_INTERVAL = 1 -- second

local metrics_batch = new_tab(MAX_BATCH_SIZE, 0)

local _M = {}

local function send(payload)
  local s = assert(socket())
  assert(s:connect("unix:/tmp/prometheus-nginx.socket"))
  assert(s:send(payload))
  assert(s:close())
end

local function metrics()
  return {
    host = ngx.var.host or "-",
    namespace = ngx.var.namespace or "-",
    ingress = ngx.var.ingress_name or "-",
    service = ngx.var.service_name or "-",
    path = ngx.var.location_path or "-",

    method = ngx.var.request_method or "-",
    status = ngx.var.status or "-",
    requestLength = tonumber(ngx.var.request_length) or -1,
    requestTime = tonumber(ngx.var.request_time) or -1,
    responseLength = tonumber(ngx.var.bytes_sent) or -1,

    --upstreamLatency = tonumber(ngx.var.upstream_connect_time) or -1,
    upstreamResponseTime = tonumber(ngx.var.upstream_response_time) or -1,
    upstreamResponseLength = tonumber(ngx.var.upstream_response_length) or -1,
    --upstreamStatus = ngx.var.upstream_status or "-",
  }
end

local function flush(premature)
  if premature then
    return
  end

  if #metrics_batch == 0 then
    return
  end

  local current_metrics_batch = clone_tab(metrics_batch)
  clear_tab(metrics_batch)

  local payload, err = cjson.encode(current_metrics_batch)
  if not payload then
    ngx.log(ngx.ERR, "error while encoding metrics: ", err)
    return
  end

  send(payload)
end

function _M.init_worker()
  local _, err = ngx.timer.every(FLUSH_INTERVAL, flush)
  if err then
    ngx.log(ngx.ERR, string.format("error when setting up timer.every: %s", tostring(err)))
  end
end

function _M.call()
  local metrics_size = nkeys(metrics_batch)
  if metrics_size >= MAX_BATCH_SIZE then
    ngx.log(ngx.WARN, "omitting metrics for the request, current batch is full")
    return
  end

  metrics_batch[metrics_size + 1] = metrics()
end

if _TEST then
  _M.flush = flush
  _M.get_metrics_batch = function() return metrics_batch end
end

return _M
