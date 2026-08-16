import request from '@/utils/request'

export interface LoginRequest {
  username: string
  password: string
}

export interface LoginResponse {
  token: string
  userId: number
  username: string
  displayName: string
  userType: string
}

export interface LoginApiResponse {
  success: boolean
  data?: LoginResponse
  message?: string
}

export const AuthApi = {
  login: (data: LoginRequest): Promise<LoginApiResponse> =>
    request.post('/auth/login', data)
}