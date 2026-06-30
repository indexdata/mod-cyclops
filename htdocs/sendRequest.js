function sendRequest(method, path, data) {
  const params = { method };
  console.log(`sendRequest: ${method} $path with data`, data);
  if (method === 'POST' || method === 'PUT') {
    params.headers = {
      'Content-Type': 'application/json'
    };
    params.body = JSON.stringify(data);
  }
  fetch(path, params)
  .then(async (response) => {
    if (!response.ok) {
      const errorText = await response.text().catch(() => '');
      alert(`Request failed with status ${response.status}: ${errorText}`);
    } else {
      const result = await response.json().catch(() => null);
      alert('Success: ' + JSON.stringify(result, null, 2));
    }
  })
  .catch((error) => {
    alert(`${method} to ${path} failed:`, error);
  });
}

function postData(path, data) {
  sendRequest('POST', path, data);
}

function putData(path, data) {
  sendRequest('PUT', path, data);
}

function getObject(path) {
  sendRequest('GET', path);
}

function deleteObject(path) {
  sendRequest('DELETE', path);
}
