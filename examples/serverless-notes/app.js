const endpointInput = document.querySelector('#endpoint');
const keyInput = document.querySelector('#key');
const connectionForm = document.querySelector('#connection-form');
const disconnectButton = document.querySelector('#disconnect');
const noteForm = document.querySelector('#note-form');
const noteText = document.querySelector('#note-text');
const saveButton = document.querySelector('#save');
const notesElement = document.querySelector('#notes');
const countElement = document.querySelector('#count');
const statusElement = document.querySelector('#status');
const lengthElement = document.querySelector('#length');
const encoder = new TextEncoder();

let endpoint = '';
let functionKey = '';
let pending = false;

const configuredEndpoint = new URLSearchParams(location.search).get('endpoint');
if (configuredEndpoint) endpointInput.value = configuredEndpoint;
if (location.hostname === '127.0.0.1' || location.hostname === 'localhost') {
  endpointInput.value ||= `${location.origin}/api/v1/run/00000000-0000-0000-0000-000000000001`;
  keyInput.value = 'local-demo';
}

function setStatus(message, error = false) {
  statusElement.textContent = message;
  statusElement.classList.toggle('error', error);
}

function setPending(value) {
  pending = value;
  saveButton.disabled = value || !functionKey;
  noteText.disabled = value || !functionKey;
  connectionForm.querySelector('button').disabled = value;
}

function validateEndpoint(value) {
  const url = new URL(value);
  const localDemo =
    ['127.0.0.1', 'localhost'].includes(location.hostname) && url.origin === location.origin;
  if (
    (url.origin !== 'https://return-api.khu-return.com' && !localDemo) ||
    !/^\/api\/v1\/run\/[0-9a-f-]{36}\/?$/.test(url.pathname) ||
    url.search ||
    url.hash
  ) {
    throw new Error('RCP 함수의 HTTP endpoint를 입력하세요.');
  }
  return url.toString().replace(/\/$/, '');
}

async function request(path, options = {}) {
  const response = await fetch(`${endpoint}${path}`, {
    ...options,
    headers: {
      Authorization: `Bearer ${functionKey}`,
      'Content-Type': 'application/json',
    },
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    if (response.status === 401) throw new Error('함수 키가 유효하지 않거나 만료됐습니다.');
    throw new Error(payload.error || `요청에 실패했습니다. (${response.status})`);
  }
  return payload;
}

function renderNotes(notes) {
  notesElement.replaceChildren();
  countElement.textContent = `${notes.length}개`;
  if (notes.length === 0) {
    setStatus('아직 메모가 없습니다. 첫 메모를 남겨 보세요.');
    return;
  }
  for (const item of notes) {
    const article = document.createElement('article');
    article.className = 'note';
    const text = document.createElement('p');
    text.className = 'note-text';
    text.textContent = item.text;
    const foot = document.createElement('div');
    foot.className = 'note-foot';
    const date = document.createElement('time');
    date.dateTime = item.createdAt;
    date.textContent = new Date(item.createdAt).toLocaleString('ko-KR');
    const remove = document.createElement('button');
    remove.type = 'button';
    remove.textContent = '삭제';
    remove.addEventListener('click', async () => {
      if (!confirm('이 메모를 삭제할까요?')) return;
      setPending(true);
      try {
        await request(`/notes/${encodeURIComponent(item.id)}`, { method: 'DELETE' });
        await loadNotes('메모를 삭제했습니다.');
      } catch (error) {
        setStatus(error.message, true);
      } finally {
        setPending(false);
      }
    });
    foot.append(date, remove);
    article.append(text, foot);
    notesElement.append(article);
  }
}

async function loadNotes(message = '') {
  const payload = await request('/notes');
  renderNotes(payload.notes || []);
  if (message) setStatus(message);
}

connectionForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  try {
    endpoint = validateEndpoint(endpointInput.value.trim());
    functionKey = keyInput.value.trim();
    if (!functionKey) throw new Error('함수 키를 입력하세요.');
    setPending(true);
    await loadNotes('함수에 연결됐습니다.');
    keyInput.value = '';
    disconnectButton.hidden = false;
  } catch (error) {
    functionKey = '';
    setStatus(error.message, true);
  } finally {
    setPending(false);
  }
});

disconnectButton.addEventListener('click', () => {
  endpoint = '';
  functionKey = '';
  keyInput.value = '';
  disconnectButton.hidden = true;
  notesElement.replaceChildren();
  countElement.textContent = '0개';
  setPending(false);
  setStatus('함수를 연결해 주세요.');
});

noteText.addEventListener('input', () => {
  lengthElement.textContent = `${encoder.encode(noteText.value.trim()).length} / 500 bytes`;
});

noteForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  if (pending) return;
  const text = noteText.value.trim();
  if (!text || encoder.encode(text).length > 500) {
    setStatus('메모는 1~500바이트로 입력하세요.', true);
    return;
  }
  setPending(true);
  try {
    await request('/notes', { method: 'POST', body: JSON.stringify({ text }) });
    noteText.value = '';
    lengthElement.textContent = '0 / 500 bytes';
    await loadNotes('메모를 저장했습니다.');
  } catch (error) {
    setStatus(error.message, true);
  } finally {
    setPending(false);
  }
});
