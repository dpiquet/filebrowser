import store from '@/store'
import router from '@/router'
import { baseURL } from '@/utils/constants'

export function parseUserData (plainData) {
  const userData = JSON.parse(plainData)
  localStorage.setItem('user_data', userData)
  store.commit('setUser', userData)
}

export async function validateLogin () {
  try {
    if (localStorage.getItem('jwt')) {
      await renew(localStorage.getItem('jwt'))
    }
  } catch (_) {
    console.warn('Invalid JWT token in storage') // eslint-disable-line
  }
}

export async function login (username, password, recaptcha) {
  const data = { "user": username, "passwd":password, "recaptcha": recaptcha }

  const res = await fetch(`${baseURL}/auth/local/login`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(data)
  })

  const body = await res.text()

  if (res.status === 200) {
    parseUserData(body)
  } else {
    throw new Error(body)
  }
}

export async function renew (jwt) {
  const res = await fetch(`${baseURL}/api/renew`, {
    method: 'POST',
    headers: {
      'X-Auth': jwt,
    }
  })

  const body = await res.text()

  if (res.status === 200) {
    //parseToken(body)
  } else {
    throw new Error(body)
  }
}

export async function signup (username, password) {
  const data = { username, password }

  const res = await fetch(`${baseURL}/api/signup`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(data)
  })

  if (res.status !== 200) {
    throw new Error(res.status)
  }
}

export function logout () {
  store.commit('setJWT', '')
  store.commit('setUser', null)
  localStorage.setItem('jwt', null)
  router.push({path: '/login'})
}
