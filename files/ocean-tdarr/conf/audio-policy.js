// Audio policy for the HEVC flow, run by tdarr's "Custom JS Function" plugin.
//
// Most viewers listen in stereo, a handful on 3.1/5.1 and nobody on Atmos, so
// every file ends with: a default "Stereo (dialogue boost)" AAC track, one AC3
// 5.1 track per language for the surround setups, and nothing lossless. AC3 is
// the surround codec that passes through on every client in the play history;
// TrueHD, DTS and AAC 5.1 mostly force a transcode or stall (Apple TV HD).
//
// Mutates args.variables.ffmpegCommand.streams; ffmpegCommandExecute turns that
// into the ffmpeg command. Output 1 always; Execute skips files with no change.
module.exports = async (args) => {
  const DIALOGUE_TITLE = 'Stereo (dialogue boost)';
  const LOSSY_SURROUND = new Set(['ac3', 'eac3']);
  const OK_STEREO = new Set(['aac', 'ac3', 'eac3', 'mp3', 'opus']);
  // mkvmerge statistics describe the stream as it was muxed. After a re-encode
  // the video's BPS still claims the source bitrate (31 Mbps on a 6 Mbps HEVC)
  // and Plex sizes transcodes from it.
  const STALE_TAGS = ['BPS', 'NUMBER_OF_BYTES', 'NUMBER_OF_FRAMES',
    '_STATISTICS_TAGS', '_STATISTICS_WRITING_APP', '_STATISTICS_WRITING_DATE_UTC'];

  const cmd = args.variables.ffmpegCommand;
  const streams = cmd.streams;
  const log = (m) => args.jobLog(`audio-policy: ${m}`);
  const tags = (s) => Object.fromEntries(Object.entries(s.tags || {}).map(([k, v]) => [k.toUpperCase(), v]));
  const lang = (s) => (tags(s).LANGUAGE || 'und').toLowerCase();
  const title = (s) => tags(s).TITLE || '';
  const isCommentary = (s) => /comment/i.test(title(s)) || (s.disposition || {}).comment === 1;
  const isLossless = (s) => ['truehd', 'flac', 'alac', 'mlp'].includes(s.codec_name)
    || s.codec_name.startsWith('pcm_')
    || (s.codec_name === 'dts' && /MA|HRA|X/.test(s.profile || ''));
  // An earlier tdarr step added a libvorbis stereo copy beside the original.
  const isTdarrVorbis = (s) => s.codec_name === 'vorbis' && /libvorbis/.test(tags(s).ENCODER || '');
  const clearStats = (s) => STALE_TAGS.forEach((t) => s.outputArgs.push('-metadata:s:{outputIndex}', `${t}=`));
  const toAc3 = (s) => clearStats(s) || s.outputArgs.push('-c:{outputIndex}', 'ac3', '-b:{outputIndex}', '640k',
    '-ac:{outputIndex}', '6', '-metadata:s:{outputIndex}', 'title=AC3 5.1');
  const toAac = (s) => clearStats(s) || s.outputArgs.push('-c:{outputIndex}', 'aac', '-b:{outputIndex}', '192k',
    '-ac:{outputIndex}', String(Math.min(s.channels || 2, 2)));

  // The flow before -c:V encoded mkv cover attachments as 30000 fps HEVC video.
  // With one in the output ffmpeg reports time=N/A, tdarr sees no progress, and
  // its stall detector cancels the job (Kiss of the Spider Woman, 2026-10-18).
  // Plex shows its own poster, so the cover goes, even from a finished file.
  const isCover = (s) => s.codec_type === 'video' && /^image\//.test(tags(s).MIMETYPE || '');
  streams.filter(isCover).forEach((s) => {
    log(`drop cover ${tags(s).FILENAME || ''} encoded as ${s.codec_name} video`);
    s.removed = true;
    cmd.shouldProcess = true;
  });

  const audio = streams.filter((s) => s.codec_type === 'audio');
  if (audio.length === 0) {
    log('no audio streams, nothing to do');
    return { outputFileObj: args.inputFileObj, outputNumber: 1, variables: args.variables };
  }
  // Files the old flow muxed twice (every stream duplicated) are being
  // re-downloaded. Same-looking audio tracks are not always copies (Bring It On
  // has two "Surround 5.1" tracks that differ), so this does not try to dedupe.
  // A second video at another size is part of the release (Chamber of Secrets
  // carries a 480p commentary picture-in-picture), not a double mux.
  const video = streams.filter((s) => s.codec_type === 'video' && !isCover(s));
  if (new Set(video.map((s) => `${s.width}x${s.height}`)).size < video.length) {
    log('two video streams at one size (double-muxed by the old flow), leaving it for a re-download');
    return { outputFileObj: args.inputFileObj, outputNumber: 1, variables: args.variables };
  }
  // English wins over a release's own default, even when that makes a dub the
  // dialogue track of a foreign film. Kiss of the Spider Woman came as a German
  // release whose default was German, beside an English AC3 5.1.
  const isEnglish = (s) => ['eng', 'en'].includes(lang(s));
  const sources = audio.filter((s) => !isCommentary(s) && title(s) !== DIALOGUE_TITLE);
  const english = sources.filter(isEnglish);
  // Idempotent: a file that already has the dialogue track was done by this
  // policy, unless it was built from another language and English is there.
  const done = audio.find((s) => title(s) === DIALOGUE_TITLE);
  if (done && (isEnglish(done) || english.length === 0)) {
    log('dialogue track present, already processed');
    return { outputFileObj: args.inputFileObj, outputNumber: 1, variables: args.variables };
  }
  if (done) {
    log(`drop ${lang(done)} ${DIALOGUE_TITLE}, rebuilding it from English`);
    done.removed = true;
    audio.splice(audio.indexOf(done), 1);
  }

  // The main track feeds the dialogue downmix: the default-flagged non-commentary
  // track, else the first, preferring English. Picked before anything is removed
  // so a lossless source still feeds it.
  const pool = english.length ? english : sources;
  const main = pool.find((s) => (s.disposition || {}).default === 1) || pool[0] || audio[0];

  const byLang = {};
  audio.filter((s) => !isCommentary(s)).forEach((s) => { (byLang[lang(s)] = byLang[lang(s)] || []).push(s); });
  Object.entries(byLang).forEach(([l, tracks]) => {
    const surround = tracks.filter((s) => (s.channels || 0) > 2);
    const keep = surround.find((s) => LOSSY_SURROUND.has(s.codec_name));
    surround.forEach((s) => {
      if (LOSSY_SURROUND.has(s.codec_name)) return;
      if (keep) {
        log(`drop ${l} ${s.codec_name} ${s.channels}ch (${l} already has ${keep.codec_name})`);
        s.removed = true;
      } else if (s === surround.find((x) => !LOSSY_SURROUND.has(x.codec_name))) {
        log(`convert ${l} ${s.codec_name} ${s.channels}ch -> ac3 5.1`);
        toAc3(s);
      } else {
        log(`drop ${l} ${s.codec_name} ${s.channels}ch (extra surround track)`);
        s.removed = true;
      }
    });
    tracks.filter((s) => (s.channels || 0) <= 2).forEach((s) => {
      if (isTdarrVorbis(s)) {
        log(`drop ${l} tdarr-added vorbis copy`);
        s.removed = true;
      } else if (!OK_STEREO.has(s.codec_name)) {
        log(`convert ${l} ${s.codec_name} ${s.channels}ch -> aac`);
        toAac(s);
      }
    });
  });
  audio.filter(isCommentary).forEach((s) => {
    if (!OK_STEREO.has(s.codec_name)) toAac(s);
  });

  // Dialogue-forward stereo: centre at full level, fronts and surrounds at 0.3,
  // then loudness-normalised to land near -18 LUFS. Single-pass loudnorm
  // undershoots over a whole film, so it aims at -16: full films measured
  // -17.8 (Amistad) and -19.2 (The Tree of Life), against -20.1 and -21.3 when
  // aimed at -18. loudnorm resamples to 192 kHz, hence the aresample to 48k.
  // Some encoders leave the layout unset; 6 and 8 channels are 5.1 and 7.1.
  const layout = main.channel_layout || { 6: '5.1', 8: '7.1' }[main.channels] || '';
  const surr = (side) => (/7\.1/.test(layout) ? `0.3*B${side}+0.3*S${side}`
    : /side/.test(layout) ? `0.3*S${side}` : `0.3*B${side}`);
  const downmix = (main.channels || 0) > 2 && /^(5\.1|7\.1)/.test(layout)
    ? `pan=stereo|FL=FC+0.3*FL+${surr('L')}|FR=FC+0.3*FR+${surr('R')}`
    : 'aformat=channel_layouts=stereo';
  const dialogue = {
    ...JSON.parse(JSON.stringify(main)),
    // NOTE: Execute resolves {outputIndex} by finding a stream's `index` in the
    // list, so a copy sharing main's index would take main's slot and the two
    // tracks' codec args would collide. ffmpeg only sees mapArgs, which keeps 0:N.
    index: 1000 + main.index,
    removed: false,
    mapArgs: ['-map', `0:${main.index}`],
    inputArgs: [],
    outputArgs: [],
  };
  clearStats(dialogue);
  dialogue.outputArgs.push('-c:{outputIndex}', 'aac', '-b:{outputIndex}', '192k',
      '-filter:{outputIndex}', `${downmix},loudnorm=I=-16:LRA=7:TP=-1.5,aresample=48000`,
      '-metadata:s:{outputIndex}', `title=${DIALOGUE_TITLE}`,
      '-metadata:s:{outputIndex}', `language=${lang(main)}`,
      '-disposition:{outputIndex}', 'default');
  log(`add ${DIALOGUE_TITLE} from ${main.codec_name} ${layout || `${main.channels}ch`} (${lang(main)})`);

  // Dialogue track first among the audio, and the only default audio track.
  audio.forEach((s) => s.outputArgs.push('-disposition:{outputIndex}', '-default'));
  const firstAudio = streams.indexOf(audio[0]);
  streams.splice(firstAudio, 0, dialogue);

  video.forEach(clearStats);

  cmd.shouldProcess = true;
  return { outputFileObj: args.inputFileObj, outputNumber: 1, variables: args.variables };
};
