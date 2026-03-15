// Fingerprint spoofing injection script.
// Injected via add_init_script() before any page content loads.
// Config is passed as __fpConfig global by the Python side.
//
// Key design principle: all noise must be DETERMINISTIC.
// Same input (canvas content, element, text) + same seed = same output.
// Fingerprint checkers call APIs multiple times to detect inconsistency.
(function () {
  "use strict";
  const C = window.__fpConfig;
  if (!C) return;

  // --- Seeded PRNG (mulberry32) ---
  function mulberry32(seed) {
    return function () {
      seed |= 0;
      seed = (seed + 0x6d2b79f5) | 0;
      let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }

  // Hide overridden functions from toString() detection.
  // Intercepts both fn.toString() and Function.prototype.toString.call(fn).
  const _overrides = new Map();
  const _origToString = Function.prototype.toString;
  Function.prototype.toString = function () {
    const s = _overrides.get(this);
    if (s !== undefined) return s;
    return _origToString.call(this);
  };
  // Hide our own toString override
  _overrides.set(
    Function.prototype.toString,
    "function toString() { [native code] }"
  );

  function hideOverride(obj, prop, fn) {
    obj[prop] = fn;
    _overrides.set(fn, "function " + prop + "() { [native code] }");
    return fn;
  }

  // Fast string hash (djb2)
  function hashStr(str) {
    let h = 5381;
    for (let i = 0; i < str.length; i++) {
      h = ((h << 5) + h + str.charCodeAt(i)) | 0;
    }
    return h;
  }

  // Fast buffer hash (samples every N-th byte for performance)
  function hashBuffer(buf, sampleStep) {
    let h = 5381;
    const step = sampleStep || 1;
    for (let i = 0; i < buf.length; i += step) {
      h = ((h << 5) + h + (buf[i] | 0)) | 0;
    }
    return h;
  }

  // Create a deterministic noise function with a specific sub-seed.
  // Calling noiseFor(sameSeed) always returns the same sequence.
  function noiseFor(subSeed) {
    const r = mulberry32((C.seed ^ subSeed) | 0);
    return function (amplitude) {
      return (r() - 0.5) * 2 * amplitude;
    };
  }

  // --- Canvas fingerprint spoofing ---
  if (C.canvas) {
    const origGetImageData =
      CanvasRenderingContext2D.prototype.getImageData;

    // Deterministic per-canvas noise: hash a sample of pixels,
    // use hash+seed to create a per-call PRNG, modify the same pixels
    // the same way every time.
    function addCanvasNoise(imageData) {
      const data = imageData.data;
      // Sample step for hashing (every ~100th byte for speed)
      const hStep = Math.max(1, Math.floor(data.length / 1000));
      const h = hashBuffer(data, hStep);
      const n = noiseFor(h);
      // Modify a subset of pixels deterministically
      const mStep = Math.max(4, Math.floor(data.length / 400));
      for (let i = 0; i < data.length; i += mStep) {
        data[i] = Math.max(0, Math.min(255, data[i] + Math.round(n(1.5))));
      }
      return imageData;
    }

    hideOverride(
      CanvasRenderingContext2D.prototype,
      "getImageData",
      function () {
        const imageData = origGetImageData.apply(this, arguments);
        return addCanvasNoise(imageData);
      }
    );

    // toDataURL: read pixels, compute deterministic noise, write back,
    // then call original. Uses a temp canvas to avoid mutating the source.
    const origToDataURL = HTMLCanvasElement.prototype.toDataURL;
    hideOverride(HTMLCanvasElement.prototype, "toDataURL", function () {
      const w = this.width;
      const h = this.height;
      if (w === 0 || h === 0) return origToDataURL.apply(this, arguments);
      // Create a clone canvas with noised pixels
      const tmp = document.createElement("canvas");
      tmp.width = w;
      tmp.height = h;
      const tmpCtx = tmp.getContext("2d");
      tmpCtx.drawImage(this, 0, 0);
      const imageData = origGetImageData.call(tmpCtx, 0, 0, w, h);
      addCanvasNoise(imageData);
      tmpCtx.putImageData(imageData, 0, 0);
      return origToDataURL.apply(tmp, arguments);
    });

    // toBlob: same approach as toDataURL
    const origToBlob = HTMLCanvasElement.prototype.toBlob;
    hideOverride(HTMLCanvasElement.prototype, "toBlob", function (callback) {
      const w = this.width;
      const h = this.height;
      if (w === 0 || h === 0) return origToBlob.apply(this, arguments);
      const tmp = document.createElement("canvas");
      tmp.width = w;
      tmp.height = h;
      const tmpCtx = tmp.getContext("2d");
      tmpCtx.drawImage(this, 0, 0);
      const imageData = origGetImageData.call(tmpCtx, 0, 0, w, h);
      addCanvasNoise(imageData);
      tmpCtx.putImageData(imageData, 0, 0);
      const args = Array.prototype.slice.call(arguments);
      args[0] = callback;
      return origToBlob.apply(tmp, args);
    });
  }

  // --- WebGL fingerprint spoofing ---
  if (C.webgl) {
    const spoofParam = function (target) {
      const origGetParameter = target.prototype.getParameter;
      hideOverride(target.prototype, "getParameter", function (param) {
        // UNMASKED_VENDOR_WEBGL = 0x9245
        if (param === 0x9245) return C.webglVendor;
        // UNMASKED_RENDERER_WEBGL = 0x9246
        if (param === 0x9246) return C.webglRenderer;
        return origGetParameter.call(this, param);
      });
    };
    if (typeof WebGLRenderingContext !== "undefined")
      spoofParam(WebGLRenderingContext);
    if (typeof WebGL2RenderingContext !== "undefined")
      spoofParam(WebGL2RenderingContext);
  }

  // --- AudioContext fingerprint spoofing ---
  if (C.audio) {
    // AnalyserNode frequency data
    const origGetFloatFrequencyData =
      AnalyserNode.prototype.getFloatFrequencyData;
    hideOverride(
      AnalyserNode.prototype,
      "getFloatFrequencyData",
      function (array) {
        origGetFloatFrequencyData.call(this, array);
        const h = hashBuffer(new Uint8Array(array.buffer), 4);
        const n = noiseFor(h);
        for (let i = 0; i < array.length; i += 3) {
          array[i] = array[i] + n(0.1);
        }
      }
    );

    const origGetByteFrequencyData =
      AnalyserNode.prototype.getByteFrequencyData;
    hideOverride(
      AnalyserNode.prototype,
      "getByteFrequencyData",
      function (array) {
        origGetByteFrequencyData.call(this, array);
        const h = hashBuffer(array, 4);
        const n = noiseFor(h);
        for (let i = 0; i < array.length; i += 3) {
          array[i] = Math.max(
            0,
            Math.min(255, array[i] + Math.round(n(1)))
          );
        }
      }
    );

    // AudioBuffer.getChannelData — used by OfflineAudioContext fingerprinting
    const origGetChannelData = AudioBuffer.prototype.getChannelData;
    hideOverride(
      AudioBuffer.prototype,
      "getChannelData",
      function (channel) {
        const data = origGetChannelData.call(this, channel);
        // Only noise the buffer once per channel (mark it)
        const key = "__fp_ch" + channel;
        if (!this[key]) {
          const h = hashBuffer(
            new Uint8Array(data.buffer, data.byteOffset, data.byteLength),
            Math.max(1, Math.floor(data.length / 500))
          );
          const n = noiseFor(h);
          for (let i = 0; i < data.length; i += 100) {
            data[i] = data[i] + n(0.0001);
          }
          this[key] = true;
        }
        return data;
      }
    );
  }

  // --- ClientRects fingerprint spoofing ---
  if (C.clientRects) {
    // Deterministic noise based on element identity (tag + original rect)
    function noiseDomRect(rect, extra) {
      const h = hashStr(
        (extra || "") +
          "|" +
          rect.x.toFixed(2) +
          "," +
          rect.y.toFixed(2) +
          "," +
          rect.width.toFixed(2) +
          "," +
          rect.height.toFixed(2)
      );
      const n = noiseFor(h);
      return new DOMRect(
        rect.x + n(0.25),
        rect.y + n(0.25),
        rect.width + n(0.15),
        rect.height + n(0.15)
      );
    }

    const origGetBCR = Element.prototype.getBoundingClientRect;
    hideOverride(
      Element.prototype,
      "getBoundingClientRect",
      function () {
        const rect = origGetBCR.call(this);
        return noiseDomRect(rect, this.tagName);
      }
    );

    const origGetCR = Element.prototype.getClientRects;
    hideOverride(Element.prototype, "getClientRects", function () {
      const rects = origGetCR.call(this);
      const tag = this.tagName;
      const result = [];
      for (let i = 0; i < rects.length; i++) {
        result.push(noiseDomRect(rects[i], tag + i));
      }
      // Mimic DOMRectList interface
      result.item = function (idx) {
        return this[idx] || null;
      };
      return result;
    });
  }

  // --- Font metrics fingerprint spoofing ---
  if (C.fonts) {
    const origMeasureText =
      CanvasRenderingContext2D.prototype.measureText;
    hideOverride(CanvasRenderingContext2D.prototype, "measureText", function (text) {
      const metrics = origMeasureText.call(this, text);
      // Deterministic: noise based on text + font
      const h = hashStr(text + "|" + this.font);
      const n = noiseFor(h);
      // Override numeric property getters on the real TextMetrics object
      // instead of using Proxy (which can be detected).
      const numericProps = [
        "width",
        "actualBoundingBoxLeft",
        "actualBoundingBoxRight",
        "fontBoundingBoxAscent",
        "fontBoundingBoxDescent",
        "actualBoundingBoxAscent",
        "actualBoundingBoxDescent",
        "alphabeticBaseline",
        "emHeightAscent",
        "emHeightDescent",
        "hangingBaseline",
        "ideographicBaseline",
      ];
      for (let i = 0; i < numericProps.length; i++) {
        const prop = numericProps[i];
        const orig = metrics[prop];
        if (typeof orig === "number") {
          const offset = (i === 0) ? n(0.2) : n(0.1);
          Object.defineProperty(metrics, prop, {
            value: orig + offset,
            writable: false,
            enumerable: true,
            configurable: true,
          });
        }
      }
      return metrics;
    });
  }

  // Clean up config from window
  try {
    delete window.__fpConfig;
  } catch (e) {
    window.__fpConfig = undefined;
  }
})();
