const {csrfToken} = window.config;

async function uploadFile(file, uploadUrl) {
  const formData = new FormData();
  formData.append('file', file, file.name);

  const res = await fetch(uploadUrl, {
    method: 'POST',
    headers: {'X-Csrf-Token': csrfToken},
    body: formData,
  });
  return await res.json();
}

function clipboardPastedImages(e) {
  if (!e.clipboardData) return [];

  const files = [];
  for (const item of e.clipboardData.items || []) {
    if (!item.type || !item.type.startsWith('image/')) continue;
    files.push(item.getAsFile());
  }

  if (files.length) {
    e.preventDefault();
    e.stopPropagation();
  }
  return files;
}


function insertAtCursor(field, value) {
  const startPos = field._data_easyMDE.codemirror.getCursor();
  if (startPos) {
    field._data_easyMDE.codemirror.setSelection(startPos, startPos);
    field._data_easyMDE.codemirror.replaceSelection(value);
  } else {
    field._data_easyMDE.value(field.value + value);
  }
}

function replaceAndKeepCursor(field, oldval, newval) {
  const startPos = field._data_easyMDE.codemirror.getCursor();
  field._data_easyMDE.value(field.value.replace(oldval, newval));
  if (startPos) {
    startPos.line += 1;
    field._data_easyMDE.codemirror.setCursor(startPos);
  }
}

export function initCompImagePaste($target) {
  const dropzone = $target[0].querySelector('.dropzone');
  if (!dropzone) {
    return;
  }
  const uploadUrl = dropzone.getAttribute('data-upload-url');
  const dropzoneFiles = dropzone.querySelector('.files');
  $(document).on('paste', '.CodeMirror', async function(e) {
    const img = clipboardPastedImages(e.originalEvent);
    const name = img[0].name.substring(0, img[0].name.lastIndexOf('.'));
    const $editor = this.CodeMirror.getTextArea();
    insertAtCursor($editor, `![${name}]()`);
    const data = await uploadFile(img[0], uploadUrl);
    replaceAndKeepCursor($editor, `![${name}]()`, `![${name}](/attachments/${data.uuid})`);
    const input = $(`<input id="${data.uuid}" name="files" type="hidden">`).val(data.uuid);
    dropzoneFiles.appendChild(input[0]);
    const upfile = {name: img[0].name, size: img[0].size, uuid: data.uuid};
    dropzone.dropzone.emit('addedfile', upfile);
    dropzone.dropzone.emit('thumbnail', upfile, `/attachments/${data.uuid}`);
    dropzone.dropzone.emit('complete', upfile);
    dropzone.dropzone.files.push(upfile);
  });
}

export function initEasyMDEImagePaste(easyMDE, dropzone, files) {
  const uploadUrl = dropzone.getAttribute('data-upload-url');
  easyMDE.codemirror.on('paste', async (_, e) => {
    for (const img of clipboardPastedImages(e)) {
      const name = img.name.substr(0, img.name.lastIndexOf('.'));
      const data = await uploadFile(img, uploadUrl);
      const pos = easyMDE.codemirror.getCursor();
      easyMDE.codemirror.replaceRange(`![${name}](/attachments/${data.uuid})`, pos);
      const input = $(`<input id="${data.uuid}" name="files" type="hidden">`).val(data.uuid);
      files.append(input);
      const upfile = {name: img.name, size: img.size, uuid: data.uuid};
      dropzone.dropzone.emit('addedfile', upfile);
      dropzone.dropzone.emit('thumbnail', upfile, `/attachments/${data.uuid}`);
      dropzone.dropzone.emit('complete', upfile);
      dropzone.dropzone.files.push(upfile);
    }
  });
}
