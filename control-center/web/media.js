// Shared H.264 + AAC WebCodecs helpers for Scout Control Center.
export function createLogger(el) {
  return {
    push(level, msg, ts) {
      const t = ts ? new Date(ts) : new Date();
      const stamp = t.toLocaleTimeString();
      if (!el) return;
      const div = document.createElement('div');
      div.className = 'log-line log-' + (level || 'info');
      div.textContent = `[${stamp}] ${msg}`;
      el.appendChild(div);
      el.scrollTop = el.scrollHeight;
      while (el.childNodes.length > 300) el.removeChild(el.firstChild);
    },
    clear() { if (el) el.innerHTML = ''; },
  };
}

const scoutLogo = new Image();
scoutLogo.src = '/scout-mark.svg';

/** Teal Scout mark + wordmark — top-left video watermark. */
export function drawScoutBrand(ctx, w, h) {
  const pad = Math.min(w, h) * 0.04;
  const size = Math.min(w, h) * 0.085;
  const x = pad;
  const y = pad;
  ctx.save();
  if (scoutLogo.complete && scoutLogo.naturalWidth > 0) {
    ctx.globalAlpha = 0.94;
    ctx.drawImage(scoutLogo, x, y, size, size);
  } else {
    // Fallback disc while SVG loads
    ctx.globalAlpha = 0.9;
    ctx.fillStyle = 'rgba(10,13,16,0.7)';
    ctx.strokeStyle = 'rgba(61,214,198,0.95)';
    ctx.lineWidth = Math.max(1.5, size * 0.04);
    ctx.beginPath();
    ctx.arc(x + size / 2, y + size / 2, size / 2 - 1, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();
    ctx.fillStyle = 'rgba(61,214,198,0.95)';
    ctx.beginPath();
    ctx.arc(x + size / 2, y + size / 2, size * 0.34, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 0.96;
  ctx.fillStyle = 'rgba(61,214,198,0.95)';
  ctx.font = `700 ${Math.max(15, size * 0.4)}px Syne, "Segoe UI", sans-serif`;
  ctx.textBaseline = 'middle';
  ctx.fillText('SCOUT', x + size + size * 0.16, y + size * 0.42);
  ctx.fillStyle = 'rgba(180,196,204,0.88)';
  ctx.font = `500 ${Math.max(9, size * 0.17)}px "IBM Plex Mono", ui-monospace, monospace`;
  ctx.fillText('CONTROL', x + size + size * 0.16, y + size * 0.72);
  ctx.restore();
}

/** Bottom-biased scout reticle + horizontal/vertical tick marks for alignment. */
export function drawReticle(ctx, w, h) {
  const cx = w / 2;
  // Sit lower in frame — closer to ground path / dock approach sight picture.
  const cy = h * 0.72;
  const lw = Math.max(1.2, w / 900);
  ctx.save();
  ctx.strokeStyle = 'rgba(61,214,198,0.9)';
  ctx.fillStyle = 'rgba(61,214,198,0.9)';
  ctx.lineWidth = lw;

  const arm = Math.min(w, h) * 0.05;
  const gap = Math.min(w, h) * 0.014;
  ctx.beginPath();
  ctx.moveTo(cx - arm, cy); ctx.lineTo(cx - gap, cy);
  ctx.moveTo(cx + gap, cy); ctx.lineTo(cx + arm, cy);
  ctx.moveTo(cx, cy - arm); ctx.lineTo(cx, cy - gap);
  ctx.moveTo(cx, cy + gap); ctx.lineTo(cx, cy + arm * 0.7);
  ctx.stroke();
  ctx.beginPath();
  ctx.arc(cx, cy, 2.2, 0, Math.PI * 2);
  ctx.fill();

  // Mil-style tick marks along bottom third horizontal
  const tickY = cy;
  const major = w * 0.08;
  const minor = major / 4;
  ctx.globalAlpha = 0.75;
  for (let i = -8; i <= 8; i++) {
    if (i === 0) continue;
    const x = cx + i * minor;
    const tall = (i % 4 === 0) ? h * 0.022 : h * 0.012;
    ctx.beginPath();
    ctx.moveTo(x, tickY - tall);
    ctx.lineTo(x, tickY + tall * 0.35);
    ctx.stroke();
  }
  // Vertical ticks on lower sides
  for (let i = -4; i <= 2; i++) {
    if (i === 0) continue;
    const y = cy + i * (h * 0.035);
    const wide = (i % 2 === 0) ? w * 0.014 : w * 0.008;
    ctx.beginPath();
    ctx.moveTo(cx - wide, y);
    ctx.lineTo(cx + wide, y);
    ctx.stroke();
  }
  ctx.globalAlpha = 1;

  // Corner brackets — skip top-left (logo) and bottom-left when mission plate is up
  const m = Math.min(w, h) * 0.05;
  const L = Math.min(w, h) * 0.035;
  const corners = [[w - m, m], [w - m, h - m]];
  if (!hasMissionInfo()) corners.push([m, h - m]);
  ctx.beginPath();
  corners.forEach(([x, y]) => {
    const sx = x > w / 2 ? -1 : 1;
    const sy = y > h / 2 ? -1 : 1;
    ctx.moveTo(x, y + sy * L); ctx.lineTo(x, y); ctx.lineTo(x + sx * L, y);
  });
  ctx.stroke();
  ctx.restore();
}

let missionInfo = { title: '', objective: '', callsign: '', notes: '' };

export function setMissionInfo(m) {
  missionInfo = {
    title: (m && m.title) || '',
    objective: (m && m.objective) || '',
    callsign: (m && m.callsign) || '',
    notes: (m && m.notes) || '',
  };
}

function hasMissionInfo() {
  return !!(missionInfo.title || missionInfo.objective || missionInfo.callsign || missionInfo.notes);
}

function ellipsize(ctx, text, maxW) {
  if (!text) return '';
  if (ctx.measureText(text).width <= maxW) return text;
  let t = text;
  while (t.length > 1 && ctx.measureText(t + '…').width > maxW) t = t.slice(0, -1);
  return t + '…';
}

/** Mission plate — bottom-left, padded so it doesn't collide with HUD brackets. */
export function drawMission(ctx, w, h) {
  if (!hasMissionInfo()) return;

  const margin = Math.min(w, h) * 0.055;
  const padX = Math.max(14, Math.min(w, h) * 0.018);
  const padY = Math.max(12, Math.min(w, h) * 0.014);
  const titleSize = Math.max(16, Math.min(w, h) * 0.026);
  const bodySize = Math.max(13, Math.min(w, h) * 0.02);
  const gap = Math.max(6, titleSize * 0.35);
  const maxInner = Math.min(w * 0.42, 420);

  const head = [missionInfo.callsign, missionInfo.title].filter(Boolean).join('  ·  ') || 'MISSION';
  const lines = [];
  if (missionInfo.objective) lines.push({ text: missionInfo.objective, kind: 'obj' });
  if (missionInfo.notes) lines.push({ text: missionInfo.notes, kind: 'note' });

  ctx.save();
  ctx.font = `700 ${titleSize}px Syne, "Segoe UI", sans-serif`;
  let innerW = ctx.measureText(head).width;
  ctx.font = `500 ${bodySize}px "IBM Plex Mono", ui-monospace, monospace`;
  for (const ln of lines) innerW = Math.max(innerW, ctx.measureText(ln.text).width);
  innerW = Math.min(maxInner, Math.max(innerW, 160));

  const blockW = innerW + padX * 2;
  const blockH = padY * 2 + titleSize + (lines.length ? gap + lines.length * (bodySize + gap * 0.85) : 0);
  const x = margin;
  const y = h - margin - blockH;

  // Panel
  ctx.fillStyle = 'rgba(8,11,14,0.72)';
  ctx.strokeStyle = 'rgba(61,214,198,0.35)';
  ctx.lineWidth = Math.max(1, Math.min(w, h) / 900);
  ctx.beginPath();
  ctx.rect(x, y, blockW, blockH);
  ctx.fill();
  ctx.stroke();

  // Accent bar
  ctx.fillStyle = 'rgba(61,214,198,0.9)';
  ctx.fillRect(x, y, 3, blockH);

  let ty = y + padY + titleSize * 0.85;
  ctx.fillStyle = 'rgba(61,214,198,0.95)';
  ctx.font = `700 ${titleSize}px Syne, "Segoe UI", sans-serif`;
  ctx.textBaseline = 'alphabetic';
  ctx.fillText(ellipsize(ctx, head, innerW), x + padX + 4, ty);

  ctx.font = `500 ${bodySize}px "IBM Plex Mono", ui-monospace, monospace`;
  for (const ln of lines) {
    ty += bodySize + gap;
    ctx.fillStyle = ln.kind === 'note' ? 'rgba(170,184,192,0.9)' : 'rgba(231,236,239,0.92)';
    ctx.fillText(ellipsize(ctx, ln.text, innerW), x + padX + 4, ty);
  }
  ctx.restore();
}

function paintOverlays(ctx, canvas, showReticle) {
  drawScoutBrand(ctx, canvas.width, canvas.height);
  drawMission(ctx, canvas.width, canvas.height);
  if (showReticle) drawReticle(ctx, canvas.width, canvas.height);
}

export function createMediaPipeline({ canvas, onStatus, reticle = true }) {
  const ctx = canvas.getContext('2d');
  let vdec = null, adec = null;
  let vConfigured = false, aConfigured = false;
  let waitingKey = true;
  let vts = 0, ats = 0;
  let audioMuted = false;
  let audioCtx = null;
  let gainNode = null;
  let nextAudioTime = 0;
  let showReticle = reticle;
  let gainValue = 6;

  function setStatus(text, ok) {
    if (onStatus) onStatus(text, ok);
  }

  function ensureGraph() {
    if (!audioCtx) audioCtx = new AudioContext();
    if (!gainNode) {
      gainNode = audioCtx.createGain();
      gainNode.gain.value = gainValue;
      gainNode.connect(audioCtx.destination);
    }
    return audioCtx;
  }

  async function ensureVideo(width, height) {
    if (!('VideoDecoder' in window)) {
      setStatus(window.isSecureContext ? 'h264: no WebCodecs' : 'h264: needs localhost', false);
      throw new Error('no VideoDecoder');
    }
    if (vdec && vConfigured) return;
    if (vdec) { try { vdec.close(); } catch (_) {} }
    vdec = new VideoDecoder({
      output: (frame) => {
        if (canvas.width !== frame.displayWidth || canvas.height !== frame.displayHeight) {
          canvas.width = frame.displayWidth;
          canvas.height = frame.displayHeight;
        }
        ctx.drawImage(frame, 0, 0, canvas.width, canvas.height);
        paintOverlays(ctx, canvas, showReticle);
        frame.close();
      },
      error: () => {
        setStatus('h264: err', false);
        vConfigured = false;
        waitingKey = true;
      },
    });
    const candidates = [
      { codec: 'avc1.640028', codedWidth: width || 1920, codedHeight: height || 1080 },
      { codec: 'avc1.4D401F', codedWidth: width || 1920, codedHeight: height || 1080 },
      { codec: 'avc1.42E01E', codedWidth: width || 1920, codedHeight: height || 1080 },
    ];
    let cfg = null;
    for (const c of candidates) {
      const full = { ...c, avc: { format: 'annexb' }, optimizeForLatency: true };
      const { supported } = await VideoDecoder.isConfigSupported(full);
      if (supported) { cfg = full; break; }
    }
    if (!cfg) { setStatus('h264: unsupported', false); throw new Error('no h264'); }
    vdec.configure(cfg);
    vConfigured = true;
    setStatus('h264: ready', true);
  }

  function parseADTS(u8) {
    if (u8.length < 7) return null;
    if (u8[0] !== 0xff || (u8[1] & 0xf0) !== 0xf0) return { raw: u8, sampleRate: 16000, channels: 1 };
    const srIdx = (u8[2] >> 2) & 0x0f;
    const rates = [96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350];
    const sampleRate = rates[srIdx] || 16000;
    const channels = ((u8[2] & 1) << 2) | ((u8[3] >> 6) & 3) || 1;
    return { raw: u8, sampleRate, channels, adts: true };
  }

  async function ensureAudio(meta) {
    if (!('AudioDecoder' in window) || audioMuted) return;
    if (adec && aConfigured) return;
    const ctxA = ensureGraph();
    if (ctxA.state === 'suspended') await ctxA.resume();
    if (adec) { try { adec.close(); } catch (_) {} }
    adec = new AudioDecoder({
      output: (audioData) => {
        try {
          const ch0 = new Float32Array(audioData.numberOfFrames);
          audioData.copyTo(ch0, { planeIndex: 0 });
          // Soft clip / normalize quiet Scout mic into usable monitor level
          let peak = 0;
          for (let i = 0; i < ch0.length; i++) {
            const a = Math.abs(ch0[i]);
            if (a > peak) peak = a;
          }
          const boost = peak > 0 && peak < 0.15 ? Math.min(4, 0.35 / peak) : 1;
          if (boost > 1.01) {
            for (let i = 0; i < ch0.length; i++) ch0[i] = Math.max(-1, Math.min(1, ch0[i] * boost));
          }
          const buf = ctxA.createBuffer(1, ch0.length, audioData.sampleRate);
          buf.copyToChannel(ch0, 0);
          const src = ctxA.createBufferSource();
          src.buffer = buf;
          src.connect(gainNode);
          const now = ctxA.currentTime;
          if (nextAudioTime < now) nextAudioTime = now + 0.02;
          src.start(nextAudioTime);
          nextAudioTime += buf.duration;
        } catch (_) {}
        audioData.close();
      },
      error: () => { aConfigured = false; },
    });
    const cfg = {
      codec: 'mp4a.40.2',
      sampleRate: meta.sampleRate || 16000,
      numberOfChannels: meta.channels || 1,
    };
    try {
      const { supported } = await AudioDecoder.isConfigSupported(cfg);
      if (!supported) return;
      adec.configure(cfg);
      aConfigured = true;
      setStatus('h264+aac: live', true);
    } catch (_) {}
  }

  return {
    setReticle(v) { showReticle = !!v; },
    setGain(v) {
      gainValue = Math.max(0.5, Math.min(16, Number(v) || 1));
      if (gainNode) gainNode.gain.setTargetAtTime(gainValue, ensureGraph().currentTime, 0.05);
    },
    setMuted(v) {
      audioMuted = !!v;
      if (audioMuted && audioCtx) audioCtx.suspend().catch(() => {});
      else if (!audioMuted && audioCtx) audioCtx.resume().catch(() => {});
    },
    async unlockAudio() {
      ensureGraph();
      await audioCtx.resume();
    },
    snapshotBlob() {
      return new Promise((resolve) => canvas.toBlob((b) => resolve(b), 'image/jpeg', 0.92));
    },
    async onPacket(buf) {
      const u8 = new Uint8Array(buf);
      if (u8.length < 2) return;
      const kind = u8[0];
      if (kind === 2) {
        const payload = u8.subarray(1);
        const meta = parseADTS(payload);
        if (!meta) return;
        try { await ensureAudio(meta); } catch (_) { return; }
        if (!adec || !aConfigured || audioMuted) return;
        ats += 20000;
        try {
          adec.decode(new EncodedAudioChunk({ type: 'key', timestamp: ats, data: payload }));
        } catch (_) { aConfigured = false; }
        return;
      }
      if (kind !== 1 || u8.length < 7) return;
      const key = u8[1] === 1;
      const width = (u8[2] << 8) | u8[3];
      const height = (u8[4] << 8) | u8[5];
      const payload = u8.subarray(6);
      if (waitingKey && !key) return;
      try { await ensureVideo(width, height); } catch (_) { return; }
      if (waitingKey && key) {
        waitingKey = false;
        setStatus('h264: live', true);
      }
      if (vdec.decodeQueueSize > 2 && !key) return;
      vts += 33333;
      try {
        vdec.decode(new EncodedVideoChunk({
          type: key ? 'key' : 'delta',
          timestamp: vts,
          data: payload,
        }));
      } catch (_) {
        waitingKey = true;
        vConfigured = false;
        setStatus('h264: wait key', false);
      }
    },
    resetKey() { waitingKey = true; },
  };
}
