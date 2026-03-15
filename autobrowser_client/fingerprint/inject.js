// Fingerprint spoofing injection script.
// Injected via add_init_script() before any page content loads.
// Config is passed as __fpConfig global by the Python side.
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
  const rng = mulberry32(C.seed);

  // Small noise value in range [-amplitude, amplitude]
  function noise(amplitude) {
    return (rng() - 0.5) * 2 * amplitude;
  }

  // --- Canvas fingerprint spoofing ---
  if (C.canvas) {
    // Noise on getImageData
    const origGetImageData =
      CanvasRenderingContext2D.prototype.getImageData;
    CanvasRenderingContext2D.prototype.getImageData = function () {
      const imageData = origGetImageData.apply(this, arguments);
      const data = imageData.data;
      // Modify a small subset of pixels for performance
      const step = Math.max(1, Math.floor(data.length / 400));
      for (let i = 0; i < data.length; i += step) {
        data[i] = Math.max(0, Math.min(255, data[i] + Math.round(noise(1.5))));
      }
      return imageData;
    };

    // Noise on toDataURL
    const origToDataURL = HTMLCanvasElement.prototype.toDataURL;
    HTMLCanvasElement.prototype.toDataURL = function () {
      const ctx = this.getContext("2d");
      if (ctx) {
        // Touch a transparent pixel to alter the hash
        const r = Math.round(rng() * 255);
        const g = Math.round(rng() * 255);
        const prev = ctx.globalAlpha;
        ctx.globalAlpha = 0.01;
        ctx.fillStyle = "rgb(" + r + "," + g + ",0)";
        ctx.fillRect(0, 0, 1, 1);
        ctx.globalAlpha = prev;
      }
      return origToDataURL.apply(this, arguments);
    };

    // Noise on toBlob
    const origToBlob = HTMLCanvasElement.prototype.toBlob;
    HTMLCanvasElement.prototype.toBlob = function (callback) {
      const ctx = this.getContext("2d");
      if (ctx) {
        const r = Math.round(rng() * 255);
        const g = Math.round(rng() * 255);
        const prev = ctx.globalAlpha;
        ctx.globalAlpha = 0.01;
        ctx.fillStyle = "rgb(" + r + "," + g + ",0)";
        ctx.fillRect(0, 0, 1, 1);
        ctx.globalAlpha = prev;
      }
      return origToBlob.apply(this, arguments);
    };
  }

  // --- WebGL fingerprint spoofing ---
  if (C.webgl) {
    const spoofParam = function (target) {
      const origGetParameter = target.prototype.getParameter;
      target.prototype.getParameter = function (param) {
        // UNMASKED_VENDOR_WEBGL = 0x9245
        if (param === 0x9245) return C.webglVendor;
        // UNMASKED_RENDERER_WEBGL = 0x9246
        if (param === 0x9246) return C.webglRenderer;
        return origGetParameter.call(this, param);
      };
    };
    if (typeof WebGLRenderingContext !== "undefined")
      spoofParam(WebGLRenderingContext);
    if (typeof WebGL2RenderingContext !== "undefined")
      spoofParam(WebGL2RenderingContext);
  }

  // --- AudioContext fingerprint spoofing ---
  if (C.audio) {
    const origGetFloatFrequencyData =
      AnalyserNode.prototype.getFloatFrequencyData;
    AnalyserNode.prototype.getFloatFrequencyData = function (array) {
      origGetFloatFrequencyData.call(this, array);
      for (let i = 0; i < array.length; i += 3) {
        array[i] = array[i] + noise(0.1);
      }
    };

    const origGetByteFrequencyData =
      AnalyserNode.prototype.getByteFrequencyData;
    AnalyserNode.prototype.getByteFrequencyData = function (array) {
      origGetByteFrequencyData.call(this, array);
      for (let i = 0; i < array.length; i += 3) {
        array[i] = Math.max(
          0,
          Math.min(255, array[i] + Math.round(noise(1)))
        );
      }
    };
  }

  // --- ClientRects fingerprint spoofing ---
  if (C.clientRects) {
    const noiseDom = function (rect) {
      const n = noise(0.5);
      return new DOMRect(
        rect.x + n,
        rect.y + n,
        rect.width + noise(0.3),
        rect.height + noise(0.3)
      );
    };

    const origGetBCR = Element.prototype.getBoundingClientRect;
    Element.prototype.getBoundingClientRect = function () {
      return noiseDom(origGetBCR.call(this));
    };

    const origGetCR = Element.prototype.getClientRects;
    Element.prototype.getClientRects = function () {
      const rects = origGetCR.call(this);
      const result = [];
      for (let i = 0; i < rects.length; i++) {
        result.push(noiseDom(rects[i]));
      }
      // Mimic DOMRectList interface
      result.item = function (idx) {
        return this[idx] || null;
      };
      return result;
    };
  }

  // --- Font metrics fingerprint spoofing ---
  if (C.fonts) {
    const origMeasureText =
      CanvasRenderingContext2D.prototype.measureText;
    CanvasRenderingContext2D.prototype.measureText = function (text) {
      const metrics = origMeasureText.call(this, text);
      const n = noise(0.2);
      // Return a proxy that adds noise to width
      return new Proxy(metrics, {
        get(target, prop) {
          if (prop === "width") return target.width + n;
          const val = target[prop];
          if (typeof val === "number") return val + noise(0.1);
          if (typeof val === "function") return val.bind(target);
          return val;
        },
      });
    };
  }

  // Clean up config from window
  try {
    delete window.__fpConfig;
  } catch (e) {
    window.__fpConfig = undefined;
  }
})();
