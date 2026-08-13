function escapeHtml (unsafe) {
  return unsafe.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;')
}

function uuidv4 () {
  if (typeof crypto !== 'undefined') {
    if (typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID()
    }

    if (typeof crypto.getRandomValues === 'function') {
      const bytes = new Uint8Array(16)
      crypto.getRandomValues(bytes)

      bytes[6] = (bytes[6] & 0x0f) | 0x40
      bytes[8] = (bytes[8] & 0x3f) | 0x80

      const hex = [...bytes].map((byte) => byte.toString(16).padStart(2, '0'))
      return `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex.slice(6, 8).join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10, 16).join('')}`
    }
  }

  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (char) => {
    const randomNibble = Math.floor(Math.random() * 16)
    const value = char === 'x' ? randomNibble : (randomNibble & 0x3) | 0x8
    return value.toString(16)
  })
}

const loadMetadataButton = document.querySelector('#load-metadata-btn')
const remoteSchemaSurveyType = document.querySelector('#remote-schema-survey-type')
const launchButton = document.querySelector('#launch-btn')
const flushButton = document.querySelector('#flush-btn')

let surveyType
let schemaUrl

function clearSurveyMetadataFields () {
  document.querySelector('#survey-type-metadata-accordion').classList.add('ons-u-vh')
  document.querySelector('#survey_metadata_fields').innerHTML = ''
  setTabIndex('survey_type_metadata_detail', -1)
}

function toggleLoadMetadataButton () {
  if (schemaUrl) {
    enableButtons([loadMetadataButton])
  } else {
    disableButtons([loadMetadataButton])
  }
}

function setSurveyType () {
  surveyType = remoteSchemaSurveyType.value
  localStorage.setItem('survey_type', surveyType) // eslint-disable-line no-undef
  setLaunchType('remote')
  toggleLoadMetadataButton()
}

function setSchemaUrl () {
  schemaUrl = document.querySelector('#remote-schema-url').value
  localStorage.setItem('schema_url', schemaUrl) // eslint-disable-line no-undef
  setLaunchType('url')
  toggleLoadMetadataButton()
}

function setLaunchType (launchType) {
  const schemaName = document.querySelector('#schema_name')
  const remoteSchemaUrl = document.querySelector('#remote-schema-url')
  const remoteSchemaSurveyType = document.querySelector('#remote-schema-survey-type')

  if (launchType === 'url') {
    clearSurveyMetadataFields()
    disableButtons([launchButton, flushButton])
    schemaName.selectedIndex = 0
    localStorage.removeItem('schema_name') // eslint-disable-line no-undef
  } else if (launchType === 'name') {
    remoteSchemaUrl.value = ''
    remoteSchemaSurveyType.selectedIndex = 0
    surveyType = null
    schemaUrl = null
    localStorage.removeItem('schema_url') // eslint-disable-line no-undef
    localStorage.removeItem('survey_type') // eslint-disable-line no-undef
    document.querySelector('#language_code').disabled = false
    disableButtons([loadMetadataButton])
  }
}

function enableButtons (buttons) {
  for (const button of buttons) {
    button.classList.remove('ons-btn--disabled')
    button.disabled = false
  }
}

function disableButtons (buttons) {
  for (const button of buttons) {
    button.classList.add('ons-btn--disabled')
    button.disabled = true
  }
}

function includeSurveyMetadataFields (schemaName, surveyTypeName) {
  const formTypeValue = schemaName.split('_').slice(1).join('_')
  document.querySelector('#survey-type-metadata-accordion').classList.remove('ons-u-vh')
  document.querySelector('.survey_heading').innerHTML = `${escapeHtml(surveyTypeName)} Survey Metadata`

  const surveyMetadataFields = document.querySelector('#survey_metadata_fields')
  const div = document.createElement('div')
  div.className = 'ons-field ons-field--inline'
  div.innerHTML = `
    <label class="ons-label" for="form_type">form_type</label>
    <input id="form_type" name="form_type" type="text" class="ons-input ons-input--text ons-input-type__input">
    `
  div.querySelector('input').value = formTypeValue
  surveyMetadataFields.textContent = ''
  surveyMetadataFields.appendChild(div)
  setTabIndex('survey_type_metadata_detail', 0)
}

function loadMetadataForSchemaName () {
  const schemaName = document.querySelector('#schema_name').value
  localStorage.setItem('schema_name', schemaName) // eslint-disable-line no-undef

  if (schemaName !== 'Select Schema') {
    const surveyType = document.querySelector(`#schema_name option[value="${schemaName}"]`).dataset.surveyType
    loadSurveyMetadata(schemaName, surveyType)
    loadSchemaMetadata(schemaName, null)
  }
}

function loadMetadataForRemoteSchema () {
  schemaUrl = document.querySelector('#remote-schema-url').value

  let schemaName = null

  if (schemaUrl && !schemaUrl.endsWith('.json')) {
    alert("Schema URL is not valid URL must end with '.json'") // eslint-disable-line no-undef
    return false
  }

  if (!remoteSchemaSurveyType.selectedIndex) {
    alert('Select a Survey Type.') // eslint-disable-line no-undef
    return false
  }

  if (!schemaUrl) {
    alert('Enter a Schema URL.') // eslint-disable-line no-undef
    return false
  }

  if (schemaUrl) {
    schemaName = schemaUrl.split('/').slice(-1)[0].split('.json')[0]
    document.querySelector('#language_code').disabled = false
  }

  loadSurveyMetadata(schemaName, remoteSchemaSurveyType.value)
  loadSchemaMetadata(schemaName, schemaUrl)
  enableButtons([launchButton, flushButton])
}

function loadSurveyMetadata (schemaName, surveyTypeName) {
  if (surveyTypeName.toLowerCase() === 'test' || surveyTypeName.toLowerCase() === 'social') {
    clearSurveyMetadataFields()
  } else {
    includeSurveyMetadataFields(schemaName, surveyTypeName)
  }
}

async function getDataAsync (queryParam) {
  return new Promise((resolve, reject) => {
    const xhttp = new XMLHttpRequest() // eslint-disable-line no-undef
    xhttp.onreadystatechange = function () {
      if (this.readyState === 4) {
        if (this.status === 200) {
          resolve(JSON.parse(this.responseText))
        } else {
          alert(`Request failed. ${this.responseText}`) // eslint-disable-line no-undef
          reject(new Error(`Request failed. ${this.responseText}`))
        }
      }
    }
    xhttp.open('GET', queryParam, true)
    xhttp.send()
  })
}

function getLabelFor (fieldName) {
  return `<label class="ons-label" for="${fieldName}">${fieldName}</label>`
}

function getInputField (fieldName, type, defaultValue = null, isReadOnly = false, onChangeCallback = null) {
  const value = defaultValue ? `value="${defaultValue}"` : ''
  const readOnly = isReadOnly ? 'readonly' : ''
  if (readOnly) {
    return `<input ${readOnly} id="${fieldName}" type="${type}" ${value} class="ons-input ons-input--text ons-input--w-20" onchange="${onChangeCallback}">`
  }
  if (type === 'checkbox') {
    return `<input ${readOnly} id="${fieldName}" type="${type}" ${value} class="ons-checkbox--toggle" onchange="${onChangeCallback}">`
  }
  return `<input ${readOnly} id="${fieldName}" name="${fieldName}" type="${type}" ${value} class="ons-input ons-input--text ons-input--w-20" onchange="${onChangeCallback}">`
}

function loadSchemaMetadata (schemaName, schemaUrl) {
  let surveyDataUrl = '/survey-data?'

  if (schemaName) surveyDataUrl += `&schema_name=${schemaName}`
  if (schemaUrl) surveyDataUrl += `&schema_url=${schemaUrl}`

  getDataAsync(surveyDataUrl)
    .then((schemaResponse) => {
      document.querySelector('#survey_metadata').innerHTML = ''

      if (schemaResponse.metadata.length > 0) {
        document.querySelector('#survey_metadata').innerHTML = schemaResponse.metadata
          .map((metadataField) => {
            const fieldName = metadataField.name
            const defaultValue = metadataField.default

            return `<div class="ons-field ons-field--inline">${getLabelFor(fieldName)}${(() => {
              if (metadataField.type === 'boolean') {
                return getInputField(fieldName, 'checkbox')
              } else if (metadataField.type === 'uuid') {
                return `<span>${getInputField(fieldName, 'text', uuidv4())}` + `<img onclick="uuid('${fieldName}')" src="data:image/svg+xml;base64,PD94bWwgdmVyc2lvbj0iMS4wIiA/PjwhRE9DVFlQRSBzdmcgIFBVQkxJQyAnLS8vVzNDLy9EVEQgU1ZHIDEuMS8vRU4nICAnaHR0cDovL3d3dy53My5vcmcvR3JhcGhpY3MvU1ZHLzEuMS9EVEQvc3ZnMTEuZHRkJz48c3ZnIGhlaWdodD0iNTEycHgiIGlkPSJMYXllcl8xIiBzdHlsZT0iZW5hYmxlLWJhY2tncm91bmQ6bmV3IDAgMCA1MTIgNTEyOyIgdmVyc2lvbj0iMS4xIiB2aWV3Qm94PSIwIDAgNTEyIDUxMiIgd2lkdGg9IjUxMnB4IiB4bWw6c3BhY2U9InByZXNlcnZlIiB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHhtbG5zOnhsaW5rPSJodHRwOi8vd3d3LnczLm9yZy8xOTk5L3hsaW5rIj48Zz48cGF0aCBkPSJNMjU2LDM4NC4xYy03MC43LDAtMTI4LTU3LjMtMTI4LTEyOC4xYzAtNzAuOCw1Ny4zLTEyOC4xLDEyOC0xMjguMVY4NGw5Niw2NGwtOTYsNTUuN3YtNTUuOCAgIGMtNTkuNiwwLTEwOC4xLDQ4LjUtMTA4LjEsMTA4LjFjMCw1OS42LDQ4LjUsMTA4LjEsMTA4LjEsMTA4LjFTMzY0LjEsMzE2LDM2NC4xLDI1NkgzODRDMzg0LDMyNywzMjYuNywzODQuMSwyNTYsMzg0LjF6Ii8+PC9nPjwvc3ZnPg==">` + '</span>'
              } else if (fieldName === 'survey_id' || fieldName === 'period_id') {
                return getInputField(fieldName, 'text', fieldName === 'survey_id' ? schemaResponse.survey_id : defaultValue, false)
              } else {
                return getInputField(fieldName, 'text', defaultValue)
              }
            })()}</div>`
          })
          .join('')
      } else {
        document.querySelector('#survey_metadata').innerHTML = 'No metadata required for this survey'
      }
      enableButtons([launchButton, flushButton])
    })
    .catch((_) => {
      document.querySelector('#survey_metadata').innerHTML = 'Failed to load Survey Metadata'
    })
}

function uuid (elementId) {
  document.querySelector(`#${elementId}`).value = uuidv4()
}

function numericId () {
  let result = ''
  const chars = '0123456789'
  for (let i = 16; i > 0; --i) {
    result += chars[Math.round(Math.random() * (chars.length - 1))]
  }
  document.querySelector('#response_id').value = result
}

function setResponseExpiry (daysOffset = 7) {
  const dt = new Date()
  dt.setDate(dt.getDate() + daysOffset)
  document.querySelector('#response_expires_at').value = dt
    .toISOString()
    .replace(/(\.\d*)/, '')
    .replace(/Z/, '+00:00')
}

function validateForm () {
  validateResponseExpiresAt()
  removeUnwantedMetadata()
}

function validateResponseExpiresAt () {
  const responseExpiresAt = Date.parse(document.querySelector('#response_expires_at').value)
  if (isNaN(responseExpiresAt)) {
    document.querySelector('#response_expires_at').remove()
  }
}

// Inputs without a name will not be submitted
function removeUnwantedMetadata () {
  const inputs = document.getElementsByTagName('input')
  for (const input of inputs) {
    if (!input.value) {
      input.removeAttribute('name')
    }
  }
}

function retrieveResponseId () {
  const responseId = localStorage.getItem('response_id') // eslint-disable-line no-undef
  const responseIdButton = document.querySelector('#response-id-btn')

  if (responseId) {
    responseIdButton.classList.remove('ons-btn--disabled')
    responseIdButton.disabled = false
  } else {
    responseIdButton.classList.add('ons-btn--disabled')
    responseIdButton.disabled = true
  }
}

function loadResponseId () {
  document.querySelector('#response_id').value = localStorage.getItem('response_id') // eslint-disable-line no-undef
}

function saveResponseId () {
  localStorage.setItem('response_id', document.querySelector('#response_id').value) // eslint-disable-line no-undef
}

function clearLocalStorage () {
  /* eslint-disable no-undef */
  localStorage.removeItem('response_id')
  localStorage.removeItem('schema_name')
  localStorage.removeItem('survey_type')
  localStorage.removeItem('schema_url')
  location.reload()
  /* eslint-enable no-undef */
}

function populateDropDownWithValue (selector, value) {
  const availableOptions = [...document.querySelector(selector).options].map((x) => x.value)

  if (availableOptions.includes(value)) {
    document.querySelector(selector).value = value
  }
}

function setTabIndex (metadataDetail, value) {
  document.getElementById(metadataDetail).tabIndex = value
}

function initialiseTabIndex () {
  const details = ['survey_type_metadata_detail']
  for (let i = 0; i < details.length; i++) {
    document.getElementById(details[i]).tabIndex = -1
  }
}

function onLoad () {
  uuid('collection_exercise_sid')
  uuid('case_id')
  numericId()
  setResponseExpiry()
  retrieveResponseId()
  initialiseTabIndex()

  const storedSchemaName = localStorage.getItem('schema_name') // eslint-disable-line no-undef
  if (storedSchemaName) {
    populateDropDownWithValue('#schema_name', storedSchemaName)
    loadMetadataForSchemaName()
  } else {
    const storedSurveyType = localStorage.getItem('survey_type') // eslint-disable-line no-undef
    if (storedSurveyType) {
      surveyType = storedSurveyType
      populateDropDownWithValue('#remote-schema-survey-type', surveyType)
    }
    const storedSchemaUrl = localStorage.getItem('schema_url') // eslint-disable-line no-undef
    if (storedSchemaUrl) {
      schemaUrl = storedSchemaUrl
      document.querySelector('#remote-schema-url').value = schemaUrl
    }
    toggleLoadMetadataButton()
  }
}

window.clearLocalStorage = clearLocalStorage
window.loadMetadataForRemoteSchema = loadMetadataForRemoteSchema
window.loadMetadataForSchemaName = loadMetadataForSchemaName
window.loadResponseId = loadResponseId
window.numericId = numericId
window.onLoad = onLoad
window.saveResponseId = saveResponseId
window.setLaunchType = setLaunchType
window.setResponseExpiry = setResponseExpiry
window.setSchemaUrl = setSchemaUrl
window.setSurveyType = setSurveyType
window.uuid = uuid
window.validateForm = validateForm
