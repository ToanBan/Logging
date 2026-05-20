from django.urls import path
from core import views

urlpatterns = [
    path('health', views.health),
    path('messages', views.get_messages),
    path('messages/room/<str:room_origin_id>', views.get_messages_by_room),
]
