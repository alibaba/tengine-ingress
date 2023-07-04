_G._TEST = true

local balancer, expected_implementations, backends
local original_ngx = ngx

local function reset_ngx()
  _G.ngx = original_ngx
end

local function mock_ngx(mock)
  local _ngx = mock
  setmetatable(_ngx, { __index = ngx })
  _G.ngx = _ngx
end

local function reset_balancer()
  package.loaded["balancer"] = nil
  balancer = require("balancer")
end

local function reset_expected_implementations()
  expected_implementations = {
    ["access-router-production-web-80"] = package.loaded["balancer.round_robin"],
    ["my-dummy-app-1"] = package.loaded["balancer.round_robin"],
    ["my-dummy-app-2"] = package.loaded["balancer.chash"],
    ["my-dummy-app-3"] = package.loaded["balancer.sticky_persistent"],
    ["my-dummy-app-4"] = package.loaded["balancer.ewma"],
    ["my-dummy-app-5"] = package.loaded["balancer.sticky_balanced"]
  }
end

local function reset_backends()
  backends = {
    {
      name = "access-router-production-web-80", port = "80", secure = false,
      sslPassthrough = false,
      endpoints = {
        { address = "10.184.7.40", port = "8080", maxFails = 0, failTimeout = 0 },
        { address = "10.184.97.100", port = "8080", maxFails = 0, failTimeout = 0 },
        { address = "10.184.98.239", port = "8080", maxFails = 0, failTimeout = 0 },
      },
      sessionAffinityConfig = { name = "", cookieSessionAffinity = { name = "" } },
      trafficShapingPolicy = {
        weight = 0,
        header = "",
        headerValue = "",
        cookie = ""
      },
    },
    { name = "my-dummy-app-1", ["load-balance"] = "round_robin", },
    {
      name = "my-dummy-app-2", ["load-balance"] = "chash",
      upstreamHashByConfig = { ["upstream-hash-by"] = "$request_uri", },
    },
    {
      name = "my-dummy-app-3", ["load-balance"] = "ewma",
      sessionAffinityConfig = { name = "cookie", mode = 'persistent', cookieSessionAffinity = { name = "route" } }
    },
    { name = "my-dummy-app-4", ["load-balance"] = "ewma", },
    {
      name = "my-dummy-app-5", ["load-balance"] = "ewma", ["upstream-hash-by"] = "$request_uri",
      sessionAffinityConfig = { name = "cookie", cookieSessionAffinity = { name = "route" } }
    },
  }
end

describe("Balancer", function()
  before_each(function()
    reset_balancer()
    reset_expected_implementations()
    reset_backends()
  end)

  after_each(function()
    reset_ngx()
  end)

  describe("get_implementation()", function()
    it("returns correct implementation for given backend", function()
      for _, backend in pairs(backends) do
        local expected_implementation = expected_implementations[backend.name]
        local implementation = balancer.get_implementation(backend)
        assert.equal(expected_implementation, balancer.get_implementation(backend))
      end
    end)
  end)

  describe("get_balancer()", function()
    it("always returns the same balancer for given request context", function()
      local backend = {
        name = "my-dummy-app-6", ["load-balance"] = "ewma",
        alternativeBackends = { "my-dummy-canary-app-6" },
        endpoints = { { address = "10.184.7.40", port = "8080", maxFails = 0, failTimeout = 0 } },
        trafficShapingPolicy = {
          weight = 0,
          header = "",
          headerValue = "",
          cookie = ""
        },
      }
      local canary_backend = {
        name = "my-dummy-canary-app-6", ["load-balance"] = "ewma",
        alternativeBackends = { "my-dummy-canary-app-6" },
        endpoints = { { address = "11.184.7.40", port = "8080", maxFails = 0, failTimeout = 0 } },
        trafficShapingPolicy = {
          weight = 5,
          header = "",
          headerValue = "",
          cookie = ""
        },
      }

      balancer.sync_backend(backend)
      balancer.sync_backend(canary_backend)

      mock_ngx({ var = { proxy_upstream_name = backend.name } })

      local expected = balancer.get_balancer()

      for i = 1,50,1 do
        assert.are.same(expected, balancer.get_balancer())
      end
    end)
  end)

  describe("route_to_alternative_balancer()", function()
    local backend, _balancer

    before_each(function()
      backend = backends[1]
      _balancer = {
        alternative_backends = {
          backend.name,
        }
      }
      mock_ngx({ var = { request_uri = "/" } })
    end)

    it("returns false when no trafficShapingPolicy is set", function()
      balancer.sync_backend(backend)
      assert.equal(false, balancer.route_to_alternative_balancer(_balancer))
    end)

    it("returns false when no alternative backends is set", function()
      backend.trafficShapingPolicy.weight = 100
      balancer.sync_backend(backend)
      _balancer.alternative_backends = nil
      assert.equal(false, balancer.route_to_alternative_balancer(_balancer))
    end)

    it("returns false when alternative backends name does not match", function()
      backend.trafficShapingPolicy.weight = 100
      balancer.sync_backend(backend)
      _balancer.alternative_backends[1] = "nonExistingBackend"
      assert.equal(false, balancer.route_to_alternative_balancer(_balancer))
    end)

    context("canary by weight", function()
      it("returns true when weight is 100", function()
        backend.trafficShapingPolicy.weight = 100
        balancer.sync_backend(backend)
        assert.equal(true, balancer.route_to_alternative_balancer(_balancer))
      end)

      it("returns false when weight is 0", function()
        backend.trafficShapingPolicy.weight = 0
        balancer.sync_backend(backend)
        assert.equal(false, balancer.route_to_alternative_balancer(_balancer))
      end)
    end)

    context("canary by cookie", function()
      it("returns correct result for given cookies", function()
        backend.trafficShapingPolicy.cookie = "canaryCookie"
        balancer.sync_backend(backend)
        local test_patterns = {
          {
            case_title = "cookie_value is 'always'",
            request_cookie_name = "canaryCookie",
            request_cookie_value = "always",
            expected_result = true,
          },
          {
            case_title = "cookie_value is 'never'",
            request_cookie_name = "canaryCookie",
            request_cookie_value = "never",
            expected_result = false,
          },
          {
            case_title = "cookie_value is undefined",
            request_cookie_name = "canaryCookie",
            request_cookie_value = "foo",
            expected_result = false,
          },
          {
            case_title = "cookie_name is undefined",
            request_cookie_name = "foo",
            request_cookie_value = "always",
            expected_result = false
          },
        }
        for _, test_pattern in pairs(test_patterns) do
          mock_ngx({ var = {
            ["cookie_" .. test_pattern.request_cookie_name] = test_pattern.request_cookie_value,
            request_uri = "/"
          }})
          assert.message("\nTest data pattern: " .. test_pattern.case_title)
            .equal(test_pattern.expected_result, balancer.route_to_alternative_balancer(_balancer))
          reset_ngx()
        end
      end)
    end)

    context("canary by header", function()
      it("returns correct result for given headers", function()
        local test_patterns = {
          -- with no header value setting
          {
            case_title = "no custom header value and header value is 'always'",
            header_name = "canaryHeader",
            header_value = "",
            request_header_name = "canaryHeader",
            request_header_value = "always",
            expected_result = true,
          },
          {
            case_title = "no custom header value and header value is 'never'",
            header_name = "canaryHeader",
            header_value = "",
            request_header_name = "canaryHeader",
            request_header_value = "never",
            expected_result = false,
          },
          {
            case_title = "no custom header value and header value is undefined",
            header_name = "canaryHeader",
            header_value = "",
            request_header_name = "canaryHeader",
            request_header_value = "foo",
            expected_result = false,
          },
          {
            case_title = "no custom header value and header name is undefined",
            header_name = "canaryHeader",
            header_value = "",
            request_header_name = "foo",
            request_header_value = "always",
            expected_result = false,
          },
          -- with header value setting
          {
            case_title = "custom header value is set and header value is 'always'",
            header_name = "canaryHeader",
            header_value = "foo",
            request_header_name = "canaryHeader",
            request_header_value = "always",
            expected_result = false,
          },
          {
            case_title = "custom header value is set and header value match custom header value",
            header_name = "canaryHeader",
            header_value = "foo",
            request_header_name = "canaryHeader",
            request_header_value = "foo",
            expected_result = true,
          },
          {
            case_title = "custom header value is set and header name is undefined",
            header_name = "canaryHeader",
            header_value = "foo",
            request_header_name = "bar",
            request_header_value = "foo",
            expected_result = false
          },
        }

        for _, test_pattern in pairs(test_patterns) do
          reset_balancer()
          backend.trafficShapingPolicy.header = test_pattern.header_name
          backend.trafficShapingPolicy.headerValue = test_pattern.header_value
          balancer.sync_backend(backend)
          mock_ngx({ var = {
            ["http_" .. test_pattern.request_header_name] = test_pattern.request_header_value,
            request_uri = "/"
          }})
          assert.message("\nTest data pattern: " .. test_pattern.case_title)
            .equal(test_pattern.expected_result, balancer.route_to_alternative_balancer(_balancer))
          reset_ngx()
        end
      end)
    end)
  end)

  describe("sync_backend()", function()
    local backend, implementation

    before_each(function()
      backend = backends[1]
      implementation = expected_implementations[backend.name]
    end)

    it("initializes balancer for given backend", function()
      local s = spy.on(implementation, "new")

      assert.has_no.errors(function() balancer.sync_backend(backend) end)
      assert.spy(s).was_called_with(implementation, backend)
    end)

    it("resolves external name to endpoints when service is of type External name", function()
      backend = {
        name = "exmaple-com", service = { spec = { ["type"] = "ExternalName" } },
        endpoints = {
          { address = "example.com", port = "80", maxFails = 0, failTimeout = 0 }
        }
      }

      helpers.mock_resty_dns_query(nil, {
        {
          name = "example.com",
          address = "192.168.1.1",
          ttl = 3600,
        },
        {
          name = "example.com",
          address = "1.2.3.4",
          ttl = 60,
        }
      })
      expected_backend = {
        name = "exmaple-com", service = { spec = { ["type"] = "ExternalName" } },
        endpoints = {
          { address = "192.168.1.1", port = "80" },
          { address = "1.2.3.4", port = "80" },
        }
      }

      local mock_instance = { sync = function(backend) end }
      setmetatable(mock_instance, implementation)
      implementation.new = function(self, backend) return mock_instance end
      assert.has_no.errors(function() balancer.sync_backend(backend) end)
      stub(mock_instance, "sync")
      assert.has_no.errors(function() balancer.sync_backend(backend) end)
      assert.stub(mock_instance.sync).was_called_with(mock_instance, expected_backend)
    end)

    it("wraps IPv6 addresses into square brackets", function()
      local backend = {
        name = "exmaple-com",
        endpoints = {
          { address = "::1", port = "8080", maxFails = 0, failTimeout = 0 },
          { address = "192.168.1.1", port = "8080", maxFails = 0, failTimeout = 0 },
        }
      }
      local expected_backend = {
        name = "exmaple-com",
        endpoints = {
          { address = "[::1]", port = "8080", maxFails = 0, failTimeout = 0 },
          { address = "192.168.1.1", port = "8080", maxFails = 0, failTimeout = 0 },
        }
      }

      local mock_instance = { sync = function(backend) end }
      setmetatable(mock_instance, implementation)
      implementation.new = function(self, backend) return mock_instance end
      assert.has_no.errors(function() balancer.sync_backend(backend) end)
      stub(mock_instance, "sync")
      assert.has_no.errors(function() balancer.sync_backend(backend) end)
      assert.stub(mock_instance.sync).was_called_with(mock_instance, expected_backend)
    end)

    it("replaces the existing balancer when load balancing config changes for backend", function()
      assert.has_no.errors(function() balancer.sync_backend(backend) end)

      backend["load-balance"] = "ewma"
      local new_implementation = package.loaded["balancer.ewma"]

      local s_old = spy.on(implementation, "new")
      local s = spy.on(new_implementation, "new")
      local s_ngx_log = spy.on(ngx, "log")

      assert.has_no.errors(function() balancer.sync_backend(backend) end)
      assert.spy(s_ngx_log).was_called_with(ngx.INFO,
      "LB algorithm changed from round_robin to ewma, resetting the instance")
      assert.spy(s).was_called_with(new_implementation, backend)
      assert.spy(s).was_called(1)
      assert.spy(s_old).was_not_called()
    end)

    it("calls sync(backend) on existing balancer instance when load balancing config does not change", function()
      local mock_instance = { sync = function(...) end }
      setmetatable(mock_instance, implementation)
      implementation.new = function(self, backend) return mock_instance end
      assert.has_no.errors(function() balancer.sync_backend(backend) end)

      stub(mock_instance, "sync")

      assert.has_no.errors(function() balancer.sync_backend(backend) end)
      assert.stub(mock_instance.sync).was_called_with(mock_instance, backend)
    end)
  end)
end)
